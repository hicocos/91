package spider91migrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/video-site/backend/internal/catalog"
	"github.com/video-site/backend/internal/drives"
	"github.com/video-site/backend/internal/drives/pikpak"
	"github.com/video-site/backend/internal/drives/spider91"
)

// fakeRegistry 是 Registry 接口的最小实现。
type fakeRegistry struct {
	byID map[string]drives.Drive
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{byID: make(map[string]drives.Drive)}
}

func (r *fakeRegistry) Add(d drives.Drive) {
	r.byID[d.ID()] = d
}

func (r *fakeRegistry) Get(id string) (drives.Drive, bool) {
	d, ok := r.byID[id]
	return d, ok
}

func (r *fakeRegistry) All() []drives.Drive {
	out := make([]drives.Drive, 0, len(r.byID))
	for _, d := range r.byID {
		out = append(out, d)
	}
	return out
}

// fakePikPak 实现 drives.Drive + pikpakUploader 接口。
type fakePikPak struct {
	id          string
	rootID      string
	uploadCalls int
	uploadFunc  func(ctx context.Context, parentID, name string, r io.Reader, size int64) (pikpak.UploadResult, error)
	mu          sync.Mutex
	gotBodies   map[string][]byte
	// renameCalls 记录每次 Rename 的 fileID->newName 历史，用于 backfill 测试断言。
	renameCalls map[string]string
}

func newFakePikPak(id, rootID string) *fakePikPak {
	return &fakePikPak{
		id:          id,
		rootID:      rootID,
		gotBodies:   make(map[string][]byte),
		renameCalls: make(map[string]string),
	}
}

func (d *fakePikPak) Kind() string { return "pikpak" }
func (d *fakePikPak) ID() string   { return d.id }
func (d *fakePikPak) RootID() string {
	return d.rootID
}
func (d *fakePikPak) Init(context.Context) error                            { return nil }
func (d *fakePikPak) List(context.Context, string) ([]drives.Entry, error)  { return nil, nil }
func (d *fakePikPak) Stat(context.Context, string) (*drives.Entry, error)   { return nil, nil }
func (d *fakePikPak) StreamURL(context.Context, string) (*drives.StreamLink, error) {
	return nil, drives.ErrNotSupported
}
func (d *fakePikPak) Upload(context.Context, string, string, io.Reader, int64) (string, error) {
	return "", drives.ErrNotSupported
}
func (d *fakePikPak) EnsureDir(context.Context, string) (string, error) {
	return "", drives.ErrNotSupported
}
func (d *fakePikPak) Rename(_ context.Context, fileID, newName string) error {
	d.mu.Lock()
	d.renameCalls[fileID] = newName
	d.mu.Unlock()
	return nil
}
func (d *fakePikPak) UploadAndReportHash(ctx context.Context, parentID, name string, r io.Reader, size int64) (pikpak.UploadResult, error) {
	d.mu.Lock()
	d.uploadCalls++
	d.mu.Unlock()
	if d.uploadFunc != nil {
		return d.uploadFunc(ctx, parentID, name, r, size)
	}
	body, _ := io.ReadAll(r)
	d.mu.Lock()
	d.gotBodies[name] = body
	d.mu.Unlock()
	return pikpak.UploadResult{
		FileID: "remote-" + name,
		Hash:   "FAKEHASH40CHARSXXXXXXXXXXXXXXXXXXXXXXXXX",
		Size:   int64(len(body)),
	}, nil
}

// 编译期断言：fakePikPak 同时满足两个接口。
var _ drives.Drive = (*fakePikPak)(nil)
var _ pikpakUploader = (*fakePikPak)(nil)

// TestBackfillFileNamesRenamesOnlyMismatchedSpider91Videos 验证回填逻辑：
//
//   - 已经是期望格式的不会再调 Rename（幂等）
//   - 名字仍是旧格式的 spider91-* 视频会被改名 + catalog 同步
//   - 不是 spider91-* 的 PikPak 视频不动（避免误伤手工导入的）
//
//   - 反复跑 runOnce 不会再重复改名
func TestBackfillFileNamesRenamesOnlyMismatchedSpider91Videos(t *testing.T) {
	cat := setupCatalog(t)
	pp := newFakePikPak("pikpak-target", "pikpak-root-id")
	reg := newFakeRegistry()
	reg.Add(pp)

	now := time.Now()

	// 1) spider91-* 视频，旧名字（viewkey.ext） → 应被改名
	stale := &catalog.Video{
		ID:            "spider91-91Spider-476fa8bf4b47e672d2fa",
		DriveID:       pp.ID(),
		FileID:        "VOtFbY2QOJdFqSx-9wPZ4rtTo2",
		FileName:      "476fa8bf4b47e672d2fa.mp4",
		Title:         "超白大奶律师约炮第一季",
		Ext:           "mp4",
		PublishedAt:   now,
		CreatedAt:     now,
		UpdatedAt:     now,
		PreviewStatus: "ready",
	}
	if err := cat.UpsertVideo(context.Background(), stale); err != nil {
		t.Fatalf("upsert stale: %v", err)
	}

	// 2) spider91-* 视频，已经是期望格式 → 应保持不动
	wantOK := desiredPikPakName("已经命名好", "abcdefgh", "mp4")
	alreadyOK := &catalog.Video{
		ID:            "spider91-91Spider-already-named-abcdefgh",
		DriveID:       pp.ID(),
		FileID:        "FILE-OK",
		FileName:      wantOK,
		Title:         "已经命名好",
		Ext:           "mp4",
		PublishedAt:   now,
		CreatedAt:     now,
		UpdatedAt:     now,
		PreviewStatus: "ready",
	}
	if err := cat.UpsertVideo(context.Background(), alreadyOK); err != nil {
		t.Fatalf("upsert ok: %v", err)
	}

	// 3) 非 spider91 的 PikPak 视频（手工上传的）→ 不应被动
	manual := &catalog.Video{
		ID:            "manual-other-id",
		DriveID:       pp.ID(),
		FileID:        "FILE-MANUAL",
		FileName:      "some random name.mp4",
		Title:         "...",
		Ext:           "mp4",
		PublishedAt:   now,
		CreatedAt:     now,
		UpdatedAt:     now,
		PreviewStatus: "ready",
	}
	if err := cat.UpsertVideo(context.Background(), manual); err != nil {
		t.Fatalf("upsert manual: %v", err)
	}

	m := New(Config{Catalog: cat, Registry: reg, GetTargetDriveID: func() string { return pp.ID() }})

	renamed, err := m.backfillFileNames(context.Background(), pp.ID(), pp)
	if err != nil {
		t.Fatalf("backfill: %v", err)
	}
	if renamed != 1 {
		t.Fatalf("renamed = %d, want 1 (only the stale spider91 video)", renamed)
	}

	// 验证 PikPak 收到的 Rename 调用：fileID = stale 的，newName = desiredPikPakName 算出
	wantStale := desiredPikPakName(stale.Title, extractViewKey(stale.ID), stale.Ext)
	if pp.renameCalls[stale.FileID] != wantStale {
		t.Errorf("rename call for %q = %q, want %q", stale.FileID, pp.renameCalls[stale.FileID], wantStale)
	}
	if _, hit := pp.renameCalls[manual.FileID]; hit {
		t.Errorf("manual upload should not be renamed; got call %q", pp.renameCalls[manual.FileID])
	}
	if _, hit := pp.renameCalls[alreadyOK.FileID]; hit {
		t.Errorf("already-named video should not be renamed; got call %q", pp.renameCalls[alreadyOK.FileID])
	}

	// catalog file_name 应被同步
	got, _ := cat.GetVideo(context.Background(), stale.ID)
	if got.FileName != wantStale {
		t.Errorf("stale catalog file_name = %q, want %q", got.FileName, wantStale)
	}

	// 第二次跑：应该 renamed=0（幂等）
	renamed2, err := m.backfillFileNames(context.Background(), pp.ID(), pp)
	if err != nil {
		t.Fatalf("backfill second time: %v", err)
	}
	if renamed2 != 0 {
		t.Errorf("second backfill renamed = %d, want 0 (should be idempotent)", renamed2)
	}
}

func keysOf(m map[string][]byte) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}

// setupCatalog 创建临时 sqlite catalog。
func setupCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.Open(filepath.Join(t.TempDir(), "video-site.db"))
	if err != nil {
		t.Fatalf("open catalog: %v", err)
	}
	t.Cleanup(func() { _ = cat.Close() })
	return cat
}

// setupSpider91 在临时目录里建一个 spider91 driver，返回 driver 和它的根目录。
func setupSpider91(t *testing.T) (*spider91.Driver, string) {
	t.Helper()
	root := t.TempDir()
	d := spider91.New(spider91.Config{ID: "spider-x", RootDir: root})
	if err := d.Init(context.Background()); err != nil {
		t.Fatalf("spider91 init: %v", err)
	}
	return d, root
}

// writeSpider91Video 在 spider91 driver 的 videos 目录下写一个 fake mp4 文件，
// 同时在 catalog 里 upsert 对应行。返回 video ID。
func writeSpider91Video(t *testing.T, cat *catalog.Catalog, d *spider91.Driver, viewkey, ext string, content []byte, publishedAt time.Time) string {
	t.Helper()
	fileID := viewkey + ext
	path, err := d.VideoPath(fileID)
	if err != nil {
		t.Fatalf("video path: %v", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	// thumb 也写一份，验证迁移后会一并删
	thumbPath, err := d.ThumbPath(viewkey + ".jpg")
	if err != nil {
		t.Fatalf("thumb path: %v", err)
	}
	if err := os.WriteFile(thumbPath, []byte("thumb"), 0o644); err != nil {
		t.Fatalf("write thumb: %v", err)
	}

	id := "spider91-" + d.ID() + "-" + viewkey
	v := &catalog.Video{
		ID:            id,
		DriveID:       d.ID(),
		FileID:        fileID,
		FileName:      fileID,
		Title:         "Sample " + viewkey,
		Author:        "tester",
		Ext:           strings.TrimPrefix(ext, "."),
		Quality:       "HD",
		Size:          int64(len(content)),
		ThumbnailURL:  "/p/thumb/" + id,
		PreviewStatus: "pending",
		PublishedAt:   publishedAt,
		CreatedAt:     publishedAt,
		UpdatedAt:     publishedAt,
	}
	if err := cat.UpsertVideo(context.Background(), v); err != nil {
		t.Fatalf("upsert video: %v", err)
	}
	return id
}

func TestRunOnceMigratesSpider91VideosAndCleansLocalFiles(t *testing.T) {
	cat := setupCatalog(t)
	src, _ := setupSpider91(t)
	pp := newFakePikPak("pikpak-target", "pikpak-root-id")

	reg := newFakeRegistry()
	reg.Add(src)
	reg.Add(pp)

	now := time.Now()
	id := writeSpider91Video(t, cat, src, "vk001", ".mp4", []byte("video bytes here"), now)

	m := New(Config{
		Catalog:          cat,
		Registry:         reg,
		GetTargetDriveID: func() string { return pp.ID() },
		Interval:         time.Hour, // 测试不靠 ticker
		KeepLatestN:      -1,        // 关闭"保留最新 N 个"，让 1 条也能立即上传
	})
	m.runOnce(context.Background())

	// 1) PikPak 收到了一次 Upload，且 parent_id 是 pikpak driver 的 RootID
	if pp.uploadCalls != 1 {
		t.Fatalf("upload calls = %d, want 1", pp.uploadCalls)
	}

	// 2) catalog 行被改写到 PikPak 上
	got, err := cat.GetVideo(context.Background(), id)
	if err != nil {
		t.Fatalf("get video: %v", err)
	}
	if got.DriveID != pp.ID() {
		t.Fatalf("drive_id = %q, want %q", got.DriveID, pp.ID())
	}
	// 上传时用的 name = desiredPikPakName(title, viewkey, ext)；
	// title="Sample vk001", viewkey="vk001"（不足 8 字符，原样返回）, ext="mp4"
	wantName := "Sample vk001-vk001.mp4"
	if _, ok := pp.gotBodies[wantName]; !ok {
		t.Fatalf("PikPak did not receive expected upload name %q (got names: %v)", wantName, keysOf(pp.gotBodies))
	}
	if got.FileID != "remote-"+wantName {
		t.Fatalf("file_id = %q, want %q", got.FileID, "remote-"+wantName)
	}
	if got.FileName != wantName {
		t.Fatalf("file_name = %q, want %q (catalog should be updated to desired name)", got.FileName, wantName)
	}
	if got.ContentHash == "" {
		t.Fatalf("content_hash should be set after migration")
	}

	// 3) 本地视频和 thumb 都被删了
	videoPath, _ := src.VideoPath("vk001.mp4")
	if _, err := os.Stat(videoPath); !os.IsNotExist(err) {
		t.Fatalf("local mp4 still exists or stat error %v", err)
	}
	thumbPath, _ := src.ThumbPath("vk001.jpg")
	if _, err := os.Stat(thumbPath); !os.IsNotExist(err) {
		t.Fatalf("local thumb still exists or stat error %v", err)
	}
}

func TestRunOnceSkipsWhenLocalFileMissing(t *testing.T) {
	cat := setupCatalog(t)
	src, _ := setupSpider91(t)
	pp := newFakePikPak("pikpak-target", "pikpak-root-id")
	reg := newFakeRegistry()
	reg.Add(src)
	reg.Add(pp)

	now := time.Now()
	id := writeSpider91Video(t, cat, src, "vk002", ".mp4", []byte("orig"), now)
	// 模拟本地文件被人手动删了
	videoPath, _ := src.VideoPath("vk002.mp4")
	_ = os.Remove(videoPath)

	m := New(Config{Catalog: cat, Registry: reg, GetTargetDriveID: func() string { return pp.ID() }})
	m.runOnce(context.Background())

	if pp.uploadCalls != 0 {
		t.Fatalf("upload calls = %d, want 0 (no local file should mean no upload)", pp.uploadCalls)
	}

	// catalog 行不应被改写
	got, _ := cat.GetVideo(context.Background(), id)
	if got.DriveID != src.ID() {
		t.Fatalf("drive_id = %q, want unchanged spider91 id %q", got.DriveID, src.ID())
	}
}

func TestRunOncePreservesStateOnUploadError(t *testing.T) {
	cat := setupCatalog(t)
	src, _ := setupSpider91(t)
	pp := newFakePikPak("pikpak-target", "pikpak-root-id")
	pp.uploadFunc = func(ctx context.Context, parentID, name string, r io.Reader, size int64) (pikpak.UploadResult, error) {
		_, _ = io.Copy(io.Discard, r) // 把字节读完，模拟到一半失败
		return pikpak.UploadResult{}, errors.New("simulated network failure")
	}
	reg := newFakeRegistry()
	reg.Add(src)
	reg.Add(pp)

	now := time.Now()
	id := writeSpider91Video(t, cat, src, "vk003", ".mp4", []byte("payload"), now)

	m := New(Config{Catalog: cat, Registry: reg, GetTargetDriveID: func() string { return pp.ID() }, KeepLatestN: -1})
	m.runOnce(context.Background())

	// 上传失败：catalog 不变 + 本地文件保留
	got, _ := cat.GetVideo(context.Background(), id)
	if got.DriveID != src.ID() {
		t.Fatalf("drive_id = %q, want still spider91 after upload failure", got.DriveID)
	}
	videoPath, _ := src.VideoPath("vk003.mp4")
	if _, err := os.Stat(videoPath); err != nil {
		t.Fatalf("local mp4 missing after failed upload: %v", err)
	}
	thumbPath, _ := src.ThumbPath("vk003.jpg")
	if _, err := os.Stat(thumbPath); err != nil {
		t.Fatalf("local thumb missing after failed upload: %v", err)
	}
}

func TestRunOnceNoOpWhenTargetDriveNotConfigured(t *testing.T) {
	cat := setupCatalog(t)
	src, _ := setupSpider91(t)
	reg := newFakeRegistry()
	reg.Add(src)

	now := time.Now()
	_ = writeSpider91Video(t, cat, src, "vk004", ".mp4", []byte("data"), now)

	m := New(Config{
		Catalog:          cat,
		Registry:         reg,
		GetTargetDriveID: func() string { return "" }, // 未配置
	})
	// 不应 panic / 不应做任何破坏性变更
	m.runOnce(context.Background())

	videoPath, _ := src.VideoPath("vk004.mp4")
	if _, err := os.Stat(videoPath); err != nil {
		t.Fatalf("local mp4 should remain when target drive unconfigured: %v", err)
	}
}

func TestRunOnceLimitsBatchSize(t *testing.T) {
	cat := setupCatalog(t)
	src, _ := setupSpider91(t)
	pp := newFakePikPak("pikpak-target", "pikpak-root-id")
	reg := newFakeRegistry()
	reg.Add(src)
	reg.Add(pp)

	now := time.Now()
	for i := 0; i < 5; i++ {
		viewkey := "vk-bulk-" + string(rune('a'+i))
		_ = writeSpider91Video(t, cat, src, viewkey, ".mp4", []byte("data"), now.Add(time.Duration(i)*time.Second))
	}

	m := New(Config{
		Catalog:          cat,
		Registry:         reg,
		GetTargetDriveID: func() string { return pp.ID() },
		BatchLimit:       2,
		// 关闭清理，否则 KeepLatestN=15 默认对 5 个文件不触发删，但显式关闭更明确
		KeepLatestN: -1,
	})
	m.runOnce(context.Background())

	if pp.uploadCalls != 2 {
		t.Fatalf("upload calls = %d, want batch limit 2", pp.uploadCalls)
	}
}

// TestCleanupRemovesAllAlreadyMigratedOrphans 验证 cleanupOldLocalVideos 的
// 新语义（防御性兜底）：
//   - 只看 catalog drive_id 是否已经迁走，不看 mtime
//   - 不依赖 KeepLatestN
//   - 已迁移的本地残留全部删除；未迁移的全部保留
//
// "保留最新 N 个本地"的语义现在归 migrateDrive 管，
// 见 TestMigrateDriveSkipsLatestNLocalFiles 等。
func TestCleanupRemovesAllAlreadyMigratedOrphans(t *testing.T) {
	cat := setupCatalog(t)
	src, _ := setupSpider91(t)
	pp := newFakePikPak("pikpak-target", "pikpak-root-id")
	reg := newFakeRegistry()
	reg.Add(src)
	reg.Add(pp)

	base := time.Now().Add(-1 * time.Hour)
	type plan struct {
		viewkey  string
		migrated bool
	}
	plans := []plan{
		{"vk-a", true},  // 已迁移 → 应被清
		{"vk-b", true},  // 已迁移 → 应被清
		{"vk-c", false}, // 未迁移 → 保留
		{"vk-d", true},  // 已迁移，即使 mtime 最新也应被清（这点跟旧语义不同）
		{"vk-e", true},  // 同上
	}
	for i, p := range plans {
		mtime := base.Add(time.Duration(i) * time.Minute)
		id := writeSpider91Video(t, cat, src, p.viewkey, ".mp4", []byte("payload-"+p.viewkey), mtime)
		path, _ := src.VideoPath(p.viewkey + ".mp4")
		_ = os.Chtimes(path, mtime, mtime)
		if p.migrated {
			if err := cat.MigrateVideoToDrive(context.Background(), id, pp.ID(), "remote-"+p.viewkey, "FAKEHASH"); err != nil {
				t.Fatalf("force-migrate %s: %v", id, err)
			}
		}
	}

	m := New(Config{
		Catalog:          cat,
		Registry:         reg,
		GetTargetDriveID: func() string { return pp.ID() },
	})

	deleted, err := m.cleanupOldLocalVideos(context.Background(), src)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if deleted != 4 {
		t.Fatalf("deleted = %d, want 4 (all migrated orphans)", deleted)
	}

	// 已迁移的 4 条都应被删；未迁移的 vk-c 应保留
	for _, p := range plans {
		path, _ := src.VideoPath(p.viewkey + ".mp4")
		_, statErr := os.Stat(path)
		exists := statErr == nil
		if p.migrated && exists {
			t.Errorf("%s migrated → should be deleted", p.viewkey)
		}
		if !p.migrated && !exists {
			t.Errorf("%s not migrated → should be retained", p.viewkey)
		}
	}
}

// TestRunOnceKeepsAllLocalWhenWithinKeepWindow 验证：本地文件数 ≤ KeepLatestN 时
// 一律不上传，全部留作"最新 N"缓存。这是用户的核心需求：刚爬下来的 15 个不要立即被传走。
func TestRunOnceKeepsAllLocalWhenWithinKeepWindow(t *testing.T) {
	cat := setupCatalog(t)
	src, _ := setupSpider91(t)
	pp := newFakePikPak("pikpak-target", "pikpak-root-id")
	reg := newFakeRegistry()
	reg.Add(src)
	reg.Add(pp)

	base := time.Now().Add(-1 * time.Hour)
	for i := 0; i < 5; i++ {
		viewkey := "vk-keep-" + string(rune('a'+i))
		_ = writeSpider91Video(t, cat, src, viewkey, ".mp4", []byte("payload-"+viewkey), base.Add(time.Duration(i)*time.Minute))
	}

	m := New(Config{
		Catalog:          cat,
		Registry:         reg,
		GetTargetDriveID: func() string { return pp.ID() },
		KeepLatestN:      15, // 本地只有 5 个 < 15，应该全部保留
	})
	m.runOnce(context.Background())

	if pp.uploadCalls != 0 {
		t.Fatalf("upload calls = %d, want 0 (5 ≤ 15 should keep all local)", pp.uploadCalls)
	}

	// 5 个本地文件都应保留
	for i := 0; i < 5; i++ {
		viewkey := "vk-keep-" + string(rune('a'+i))
		path, _ := src.VideoPath(viewkey + ".mp4")
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s should be retained: %v", viewkey, err)
		}
	}
}

// TestRunOnceMigratesOnlyOlderFilesBeyondKeepWindow 验证：本地文件数 > KeepLatestN 时
// 按 mtime 降序保留最新 N 个，超出部分（更旧的）才上传到 PikPak。
func TestRunOnceMigratesOnlyOlderFilesBeyondKeepWindow(t *testing.T) {
	cat := setupCatalog(t)
	src, _ := setupSpider91(t)
	pp := newFakePikPak("pikpak-target", "pikpak-root-id")
	reg := newFakeRegistry()
	reg.Add(src)
	reg.Add(pp)

	base := time.Now().Add(-1 * time.Hour)
	// 写 20 个本地文件，mtime 递增（i=0 最旧, i=19 最新）
	type planEntry struct {
		index    int
		viewkey  string
		expected string // "migrated" 表示应被传走 / "kept" 表示应保留
	}
	plans := make([]planEntry, 20)
	for i := 0; i < 20; i++ {
		viewkey := fmt.Sprintf("vk-batch-%02d", i)
		mtime := base.Add(time.Duration(i) * time.Minute)
		_ = writeSpider91Video(t, cat, src, viewkey, ".mp4", []byte("payload-"+viewkey), mtime)
		path, _ := src.VideoPath(viewkey + ".mp4")
		_ = os.Chtimes(path, mtime, mtime)
		// 最新 15 个保留，最旧 5 个上传
		expected := "migrated"
		if i >= 5 {
			expected = "kept"
		}
		plans[i] = planEntry{index: i, viewkey: viewkey, expected: expected}
	}

	m := New(Config{
		Catalog:          cat,
		Registry:         reg,
		GetTargetDriveID: func() string { return pp.ID() },
		KeepLatestN:      15,
	})
	m.runOnce(context.Background())

	if pp.uploadCalls != 5 {
		t.Fatalf("upload calls = %d, want 5 (oldest 5 of 20 should migrate)", pp.uploadCalls)
	}

	// 验证每条预期
	for _, p := range plans {
		path, _ := src.VideoPath(p.viewkey + ".mp4")
		_, statErr := os.Stat(path)
		exists := statErr == nil
		switch p.expected {
		case "migrated":
			if exists {
				t.Errorf("%s (idx=%d, oldest 5) should be migrated and local removed", p.viewkey, p.index)
			}
			// catalog 应改成 PikPak
			id := "spider91-" + src.ID() + "-" + p.viewkey
			v, _ := cat.GetVideo(context.Background(), id)
			if v.DriveID != pp.ID() {
				t.Errorf("%s drive_id = %q, want PikPak after migration", p.viewkey, v.DriveID)
			}
		case "kept":
			if !exists {
				t.Errorf("%s (idx=%d, newest 15) should be retained locally", p.viewkey, p.index)
			}
			id := "spider91-" + src.ID() + "-" + p.viewkey
			v, _ := cat.GetVideo(context.Background(), id)
			if v.DriveID != src.ID() {
				t.Errorf("%s drive_id = %q, want spider91 (still local)", p.viewkey, v.DriveID)
			}
		}
	}
}

