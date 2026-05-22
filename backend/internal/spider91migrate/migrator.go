// Package spider91migrate 周期性把 spider91 drive 下载到本地的视频
// 上传到一个指定的 PikPak drive 目录，上传成功后：
//
//   - 改写 catalog 行：drive_id / file_id / content_hash 改成 PikPak 的；
//     视频自身的 id 不变（仍是 spider91-<driveID>-<viewkey>），video_tags、
//     收藏、点赞、views 等关联数据全部保留
//   - 删除本地 mp4（spider91/<id>/videos/<viewkey>.<ext>）和 thumb（spider91/<id>/thumbs/<viewkey>.jpg）
//
// 之后回放时，videoSource() 自动落到 /p/stream/<pikpak>/<file_id>，
// proxy 层走 302 直连 PikPak CDN。
//
// 下次 PikPak 扫盘时，scanner 通过 (content_hash) / (file_name+size)
// 已有的 findDuplicate 兜底逻辑，不会为同一物理文件再建一行。
package spider91migrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/video-site/backend/internal/catalog"
	"github.com/video-site/backend/internal/drives"
	"github.com/video-site/backend/internal/drives/pikpak"
	"github.com/video-site/backend/internal/drives/spider91"
)

// pikpakUploader 是 migrator 对 PikPak driver 的最小依赖接口。
// *pikpak.Driver 实现了它；测试可以注入 fake。
type pikpakUploader interface {
	ID() string
	Kind() string
	RootID() string
	UploadAndReportHash(ctx context.Context, parentID, name string, r io.Reader, size int64) (pikpak.UploadResult, error)
	Rename(ctx context.Context, fileID, newName string) error
}

// Registry 是 worker 用来按 driveID 取 driver 的最小依赖。
type Registry interface {
	Get(id string) (drives.Drive, bool)
	All() []drives.Drive
}

type Config struct {
	Catalog          *catalog.Catalog
	Registry         Registry
	GetTargetDriveID func() string // 通常对应 App.Spider91UploadDriveID()
	Interval         time.Duration // 0 时默认 60s
	BatchLimit       int           // 单轮最多迁多少个，0 时默认 50
	// KeepLatestN 是每个 spider91 drive 在本地保留的最新视频数。
	// 超过的部分中"已迁移"的会被清理；未迁移的不动。0 时默认 15；< 0 关闭清理。
	KeepLatestN int
	OnMigrated  func(videoID string)
}

type Migrator struct {
	cfg     Config
	trigger chan struct{}
	mu      sync.Mutex
	running bool
}

func New(cfg Config) *Migrator {
	if cfg.Interval == 0 {
		cfg.Interval = 60 * time.Second
	}
	if cfg.BatchLimit == 0 {
		cfg.BatchLimit = 50
	}
	if cfg.KeepLatestN == 0 {
		cfg.KeepLatestN = 15
	}
	return &Migrator{
		cfg:     cfg,
		trigger: make(chan struct{}, 1),
	}
}

// Trigger 安排一次"立即跑"。多次调用会被合并成一次（channel buffer=1）。
func (m *Migrator) Trigger() {
	select {
	case m.trigger <- struct{}{}:
	default:
	}
}

// Run 是阻塞循环；ctx 取消时退出。
func (m *Migrator) Run(ctx context.Context) {
	t := time.NewTicker(m.cfg.Interval)
	defer t.Stop()
	// 启动后立刻跑一次（不等第一个 tick）
	m.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.runOnce(ctx)
		case <-m.trigger:
			m.runOnce(ctx)
		}
	}
}

// runOnce 单轮：扫所有 spider91 drive，对每条还有本地文件的视频做迁移。
//
// 互斥保证：同一 Migrator 内不会并发跑两轮（避免重复上传）。
func (m *Migrator) runOnce(ctx context.Context) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	target, pp, err := m.resolveTarget()
	if err != nil {
		// 没目标就静默 —— 用户可能还没配 PikPak drive
		return
	}

	migrated := 0
	for _, src := range m.spider91Drives() {
		if err := ctx.Err(); err != nil {
			return
		}
		n, err := m.migrateDrive(ctx, src, target, pp)
		if err != nil {
			log.Printf("[spider91migrate] drive=%s migrate batch error: %v", src.ID(), err)
		}
		migrated += n
	}
	if migrated > 0 {
		log.Printf("[spider91migrate] migrated %d video(s) to drive=%s", migrated, target)
	}

	// 收尾：扫每个 spider91 drive 的本地目录，把 catalog 已经迁到别处但本地
	// 仍有残留的孤儿文件清掉。这是纯防御性兜底——正常路径下 migrateDrive
	// 已经在迁移成功后立刻 CleanupSpider91Local，不会留孤儿。
	for _, src := range m.spider91Drives() {
		if err := ctx.Err(); err != nil {
			return
		}
		deleted, err := m.cleanupOldLocalVideos(ctx, src)
		if err != nil {
			log.Printf("[spider91migrate] cleanup drive=%s: %v", src.ID(), err)
		}
		if deleted > 0 {
			log.Printf("[spider91migrate] cleanup drive=%s deleted %d orphan local file(s)", src.ID(), deleted)
		}
	}

	// 回填：把已迁移到 PikPak 的 spider91-* 视频里文件名仍是旧格式
	// （比如刚迁完没改、或人工导入）的统一改成方案 B 期望的格式。
	// 这一步幂等：已经是期望格式的不会再调 Rename。
	if renamed, err := m.backfillFileNames(ctx, target, pp); err != nil {
		log.Printf("[spider91migrate] backfill names: %v", err)
	} else if renamed > 0 {
		log.Printf("[spider91migrate] backfilled %d PikPak file name(s) to desired format", renamed)
	}
}

// resolveTarget 返回 (PikPak drive ID, PikPak driver, err)。
// 没设置或 drive 找不到时返回 err（调用方静默跳过）。
func (m *Migrator) resolveTarget() (string, pikpakUploader, error) {
	if m.cfg.GetTargetDriveID == nil {
		return "", nil, errors.New("no target getter")
	}
	id := m.cfg.GetTargetDriveID()
	if id == "" {
		return "", nil, errors.New("target drive not configured")
	}
	d, ok := m.cfg.Registry.Get(id)
	if !ok {
		return "", nil, fmt.Errorf("target drive %q not in registry", id)
	}
	pp, ok := d.(pikpakUploader)
	if !ok {
		return "", nil, fmt.Errorf("target drive %q kind=%s, want pikpak", id, d.Kind())
	}
	return id, pp, nil
}

// spider91Drives 返回当前注册的所有 spider91 driver。
func (m *Migrator) spider91Drives() []*spider91.Driver {
	all := m.cfg.Registry.All()
	out := make([]*spider91.Driver, 0, len(all))
	for _, d := range all {
		if d.Kind() != spider91.Kind {
			continue
		}
		if sd, ok := d.(*spider91.Driver); ok {
			out = append(out, sd)
		}
	}
	return out
}

// migrateDrive 对单个 spider91 drive 跑一批迁移；返回成功迁移的条数。
//
// 策略（与"本地缓存最新 N 个"语义一致）：
//   - 列出 spider91 drive 本地 videos/ 目录所有 mp4 文件，按 mtime 降序排
//   - 跳过最新 KeepLatestN 个：这些是用户希望保留在本地的最新爬取
//   - 对剩下的（更旧）逐个处理：
//     * 还没迁移（drive_id 仍是 src.ID()）→ 上传到 PikPak + 改 catalog + 删本地
//     * 已经迁移过但本地还有残留 → 仅删本地（兜底）
//
// KeepLatestN < 0 时不保护任何本地文件，全部尝试迁移（旧行为，主要给测试用）。
func (m *Migrator) migrateDrive(ctx context.Context, src *spider91.Driver, targetDriveID string, pp pikpakUploader) (int, error) {
	keepN := m.cfg.KeepLatestN
	if keepN < 0 {
		keepN = 0
	}

	type localFile struct {
		name    string
		modTime time.Time
	}

	entries, err := os.ReadDir(src.VideosDir())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read videos dir: %w", err)
	}

	files := make([]localFile, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, localFile{name: e.Name(), modTime: info.ModTime()})
	}

	// 本地数量没超过 keepN 时不动任何文件 —— 这条是 KeepLatestN 语义的核心
	if m.cfg.KeepLatestN >= 0 && len(files) <= keepN {
		return 0, nil
	}

	// 按 mtime 降序：最新的排前面，保留前 keepN 个
	sort.Slice(files, func(i, j int) bool { return files[i].modTime.After(files[j].modTime) })

	// 候选 = 跳过最新 keepN 个之外的（更旧的）。KeepLatestN < 0 时 candidates=files。
	skip := keepN
	if m.cfg.KeepLatestN < 0 {
		skip = 0
	}
	candidates := files
	if skip < len(files) {
		candidates = files[skip:]
	} else {
		return 0, nil
	}

	migrated := 0
	for _, f := range candidates {
		if err := ctx.Err(); err != nil {
			return migrated, err
		}
		if migrated >= m.cfg.BatchLimit {
			break
		}

		viewkey := stripExt(f.name)
		videoID := "spider91-" + src.ID() + "-" + viewkey
		v, err := m.cfg.Catalog.GetVideo(ctx, videoID)
		if err != nil || v == nil {
			// 找不到 catalog 行：保险起见保留本地，让管理员可见
			continue
		}

		if v.DriveID != src.ID() {
			// catalog 已迁移到别的 drive，但本地还有残留 → 兜底删本地
			CleanupSpider91Local(src, v.FileID)
			continue
		}

		ok, err := m.migrateOne(ctx, v, src, targetDriveID, pp)
		if err != nil {
			log.Printf("[spider91migrate] %s: %v", v.ID, err)
			continue
		}
		if ok {
			migrated++
			if m.cfg.OnMigrated != nil {
				m.cfg.OnMigrated(v.ID)
			}
		}
	}
	return migrated, nil
}

// migrateOne 把单条 spider91 视频上传到 PikPak 并改写 catalog。
// 返回 (true, nil) 表示真的迁了一条；(false, nil) 表示跳过（本地文件已不在等）；
// (false, err) 表示真出错。
func (m *Migrator) migrateOne(ctx context.Context, v *catalog.Video, src *spider91.Driver, targetDriveID string, pp pikpakUploader) (bool, error) {
	path, err := src.VideoPath(v.FileID)
	if err != nil {
		return false, fmt.Errorf("resolve local path: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 本地文件被人手动删了，但 catalog 还显示 spider91 drive；
			// 这种状态没法迁移。跳过即可（保留行让管理员可见，避免数据丢失）。
			return false, nil
		}
		return false, fmt.Errorf("stat local: %w", err)
	}
	if info.IsDir() || info.Size() == 0 {
		return false, fmt.Errorf("local file invalid: dir=%v size=%d", info.IsDir(), info.Size())
	}

	f, err := os.Open(path)
	if err != nil {
		return false, fmt.Errorf("open local: %w", err)
	}
	defer f.Close()

	// 上传到 PikPak 的根目录（用户配置的 PikPak drive 的 rootID）。
	// 上传名走 desiredPikPakName 算出来的方案 B 格式：
	//
	//   <sanitized title>-<viewkey 后 8 位>.<ext>
	//
	// 这样 PikPak Web 端列出来的文件名能直接看出是哪个视频，
	// 又用 viewkey 后 8 位避免同标题撞名。
	parent := pp.RootID()
	uploadName := desiredPikPakName(v.Title, extractViewKey(v.ID), v.Ext)
	res, err := pp.UploadAndReportHash(ctx, parent, uploadName, f, info.Size())
	if err != nil {
		return false, fmt.Errorf("pikpak upload: %w", err)
	}
	if res.FileID == "" {
		return false, errors.New("pikpak returned empty file id")
	}

	// 事务性改写 catalog 行：drive_id / file_id / content_hash
	if err := m.cfg.Catalog.MigrateVideoToDrive(ctx, v.ID, targetDriveID, res.FileID, res.Hash); err != nil {
		return false, fmt.Errorf("catalog migrate: %w", err)
	}
	// 同步 catalog 里的 file_name，让下次 PikPak 扫盘时 (file_name, size) 也能匹配上
	if err := m.cfg.Catalog.UpdateVideoMeta(ctx, v.ID, catalog.VideoMetaPatch{FileName: uploadName}); err != nil {
		log.Printf("[spider91migrate] %s update file_name after migrate: %v", v.ID, err)
	}

	// 删除本地 mp4 和 thumb（thumb 在 previews/thumbs/ 还有副本，不影响展示）
	CleanupSpider91Local(src, v.FileID)

	log.Printf("[spider91migrate] %s migrated to drive=%s file=%s name=%q", v.ID, targetDriveID, res.FileID, uploadName)
	return true, nil
}

// CleanupSpider91Local 删除已迁移视频的本地 mp4 和 thumb。
//
// thumb 删除是 best-effort —— 找不到就算了（spider91 thumb 文件名带后缀，
// 我们不知道具体是 .jpg 还是别的，逐个尝试常见后缀）。
//
// 暴露成包级函数方便 cleanup 模块复用（任务 6）。
func CleanupSpider91Local(src *spider91.Driver, fileID string) {
	videoPath, err := src.VideoPath(fileID)
	if err == nil {
		if err := os.Remove(videoPath); err != nil && !os.IsNotExist(err) {
			log.Printf("[spider91migrate] remove local mp4 %s: %v", videoPath, err)
		}
	}
	// thumb 文件名是 <viewkey>.<ext>；fileID 是 <viewkey>.<videoExt>，
	// 不一定相同。尝试用 fileID 去掉视频扩展名后拼 thumb 常见后缀。
	thumbBase := stripExt(fileID)
	for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
		thumbPath, err := src.ThumbPath(thumbBase + ext)
		if err != nil {
			continue
		}
		_ = os.Remove(thumbPath) // 忽略错误：找不到很正常
	}
}

func stripExt(name string) string {
	ext := filepath.Ext(name)
	return name[:len(name)-len(ext)]
}

// cleanupOldLocalVideos 是防御性兜底：扫 spider91 drive 本地 videos/ 目录，
// 删除所有 catalog 中已经迁移到别处（drive_id != src.ID()）的本地残留。
//
// 与 migrateDrive 的区别：
//   - 不上传任何东西
//   - 不依赖 KeepLatestN —— 哪怕这个孤儿在"最新 N"窗口内，已迁移就该删
//   - 只看 catalog 状态，不看 mtime
//
// 正常路径下 migrateDrive 迁移成功后立刻 CleanupSpider91Local，所以这里
// 应该不会有任何工作。极端情况（手工改 catalog、迁移过程中 crash）才会
// 找到孤儿。
//
// 返回实际删除的文件个数。
func (m *Migrator) cleanupOldLocalVideos(ctx context.Context, src *spider91.Driver) (int, error) {
	entries, err := os.ReadDir(src.VideosDir())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	deleted := 0
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return deleted, err
		}
		if e.IsDir() {
			continue
		}
		viewkey := stripExt(e.Name())
		videoID := "spider91-" + src.ID() + "-" + viewkey
		v, err := m.cfg.Catalog.GetVideo(ctx, videoID)
		if err != nil || v == nil {
			// 找不到 catalog 行：保险起见保留，等管理员处理
			continue
		}
		if v.DriveID == src.ID() {
			// 还没迁移，归 migrateDrive 管，不在这里动
			continue
		}
		// 已迁移到别的 drive 但本地还有 → 删
		path, perr := src.VideoPath(e.Name())
		if perr != nil {
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("[spider91migrate] cleanup remove %s: %v", path, err)
			continue
		}
		// thumb 一并删（best-effort）
		thumbBase := stripExt(e.Name())
		for _, ext := range []string{".jpg", ".jpeg", ".png", ".webp"} {
			tp, terr := src.ThumbPath(thumbBase + ext)
			if terr != nil {
				continue
			}
			_ = os.Remove(tp)
		}
		deleted++
	}
	return deleted, nil
}

// backfillFileNames 扫描 PikPak 目标 drive 下所有 spider91-* 起始 ID 的视频，
// 对文件名不是 desiredPikPakName(...) 期望格式的，调 PikPak.Rename 修正，
// 并把 catalog.file_name 同步到新名字。
//
// 幂等：已经是期望格式的视频不会触发任何调用。
//
// 返回成功改名的条数。
func (m *Migrator) backfillFileNames(ctx context.Context, targetDriveID string, pp pikpakUploader) (int, error) {
	videos, err := m.cfg.Catalog.ListVideosByDriveID(ctx, targetDriveID, 10000)
	if err != nil {
		return 0, fmt.Errorf("list videos: %w", err)
	}
	renamed := 0
	for _, v := range videos {
		if err := ctx.Err(); err != nil {
			return renamed, err
		}
		if !strings.HasPrefix(v.ID, "spider91-") {
			continue
		}
		want := desiredPikPakName(v.Title, extractViewKey(v.ID), v.Ext)
		if v.FileName == want {
			continue
		}
		if v.FileID == "" {
			continue
		}
		if err := pp.Rename(ctx, v.FileID, want); err != nil {
			log.Printf("[spider91migrate] rename %s -> %q: %v", v.ID, want, err)
			continue
		}
		if err := m.cfg.Catalog.UpdateVideoMeta(ctx, v.ID, catalog.VideoMetaPatch{FileName: want}); err != nil {
			log.Printf("[spider91migrate] %s update file_name after rename: %v", v.ID, err)
			// PikPak 已经改名成功，但 catalog 更新失败 —— 下轮会重试。继续。
		}
		log.Printf("[spider91migrate] renamed %s on PikPak: %q -> %q", v.ID, v.FileName, want)
		renamed++
	}
	return renamed, nil
}
