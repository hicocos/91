# 存储 Provider 添加链路重构：会话交接

> 更新时间：2026-07-20
> 项目：`/www/91`（GitHub `hicocos/91`）
> 当前分支：`main`，本地领先 `origin/main` 16 个提交
> 状态：2026-07-20 02:11 UTC 已完成替代性独立复审的全部 4 个 Important 与 2 个 Minor 修复，并重新构建部署镜像 `sha256:caf2bf2d...`，容器 healthy。最新 Go test/vet/race、前端 lint/242 tests/build、Compose、`git diff --check` 与部署后 smoke 均通过。尚未 commit、未 push；真实第三方账户验证仍需凭据。

## 1. 用户最终确认的范围

用户最初要求完善 OneDrive、Google Drive、WebDAV、S3 的添加、授权、测试后保存与安全链路。过程中用户明确修正上传范围：

- **只删除本次新增的上传能力。**
- 新增的 S3 provider 是只读媒体来源，不需要上传或创建目录。
- 项目原来已有的 OneDrive、Google Drive、WebDAV 上传、`EnsureDir`、爬虫迁移上传目标、转码产物写回全部保留，不得删除或改变。
- S3 保留：添加、连接测试、目录扫描、Stat、播放、管理员显式删除源文件。
- S3 不进入 crawler upload target；`Upload` / `EnsureDir` 只返回 `drives.ErrNotSupported`，不得发送远端请求。
- 未经用户明确要求，不得 push GitHub。本轮用户没有要求 push。

## 2. 本轮已经完成的主要功能

### 2.1 Provider registry 与 manifest

新增 `backend/internal/storageproviders/`：

- 统一 provider descriptor/manifest。
- OneDrive、Google Drive、WebDAV、S3 提供字段、认证方式、root 模式和能力声明。
- S3 manifest 明确 `upload=false`，保留 list/play/delete。
- legacy provider 继续回退到原前端常量和原业务流程。

前端：

- `DrivesPage` 调用 `api.listStorageProviders()`。
- 新增与编辑两个 `DriveForm` 调用都传入当前 `providerManifest`。
- 四个重点 provider 的字段由 manifest 映射；legacy provider 保持旧的 QR/自定义表单。
- 新四类编辑改用脱敏 `GET /storage/accounts/{id}`，不再读取明文 secret。

### 2.2 后端绑定的 probe-before-save

新增：

- `GET /admin/api/storage/providers`
- `POST /admin/api/storage/accounts/probe`
- `POST /admin/api/storage/accounts`
- `PUT /admin/api/storage/accounts/{id}`
- `GET /admin/api/storage/accounts/{id}`

行为：

- 保存接口在后端合并保留的敏感字段后，对最终 candidate 执行 probe，再写 DB。
- probe 失败时不修改原行。
- Secret 编辑返回空字符串与 `configured` 状态；留空表示沿用旧值。
- DB 保存后若 runtime attach 失败：
  - 编辑账户恢复完整旧行；
  - 新建账户删除失败残留行；
  - 返回 502，不再把挂载失败包装为保存成功 warning。

### 2.3 S3 只读 Driver

新增 `backend/internal/drives/s3/`：

- 标准库实现 SigV4。
- ListObjectsV2 + Prefix + Delimiter。
- HeadObject/Stat。
- presigned GET 播放。
- DeleteObject（管理员显式删除源文件）。
- path-style / virtual-host 风格。
- session token 正确纳入 SignedHeaders。
- object ETag 不冒充可靠内容 MD5。
- root prefix fail-closed：
  - 未配置 root prefix 时禁止列整个 bucket 根；
  - root 外 List/Stat 在发 HTTP 前报错；
  - `root/` 与 `rooted/` 不会混淆。
- `Upload` / `EnsureDir` 返回 `drives.ErrNotSupported` 且不读 body、不发网络。
- 不加入 crawler upload target。

### 2.4 保留原有上传能力

曾有并行实现错误地把所有 provider 改为只读并拆公共接口，后来已按用户澄清修正：

- OneDrive、Google Drive、WebDAV 原 Upload/EnsureDir 保留。
- 前端 `CrawlersPage` 的原上传目标集合已恢复 OneDrive、Google Drive、WebDAV。
- 后端 `isCrawlerUploadTargetKind` 也恢复这三类。
- transcode 通过 optional `drives.Uploader` / `drives.DirectoryEnsurer` 使用原上传路径。
- S3 不实现这些 optional capability（或仅最小 ErrNotSupported 兼容，需以当前源码为准），不会远端写入。

### 2.5 OneDrive

`backend/internal/drives/onedrive/driver.go` 已补：

- `client_id`、`client_secret`、tenant。
- 直接调用 Microsoft token endpoint 刷新，不把第三方 renew API 当新账号默认方案。
- SharePoint `site_id` + `drive_id` 上下文。
- probe/attach 全链路传递上述字段。
- token callback 改为 catalog 字段级 merge，不再完整 Upsert 旧 Drive 快照。

### 2.6 Google Drive / Shared Drive / Shortcut

已补：

- `shared_drive_id` 进入 Config、probe、attach。
- List 使用：
  - `corpora=drive`
  - `driveId=...`
  - `includeItemsFromAllDrives=true`
  - `supportsAllDrives=true`
- shortcut 的 catalog `Entry.ID` 保留 shortcut 自身 ID，因此 Remove 删除 shortcut 而不是目标文件。
- `StreamURL` 获取 shortcut metadata 后对目标 ID 生成媒体 URL，因此视频 shortcut 可播放。
- 文件夹 shortcut 在递归 List 时解析为 target parent ID，同时保留 shortcut ID 作为 catalog/删除语义。
- scanner 在进入目录前检查 `VisitedDirIDs`，防止 shortcut 指向祖先/root 形成无限递归环。

### 2.7 OAuth start/callback + popup

新增后端：

- `POST /admin/api/storage/oauth/{provider}/start`
- `GET /admin/api/storage/oauth/{provider}/callback`
- `GET /admin/api/storage/oauth/callback.js`

支持 provider：OneDrive、Google Drive。

安全行为：

- 随机 state/nonce。
- state 只存 hash。
- 绑定管理员 session identity、provider、redirect URI。
- pending config 用 AES-GCM 加密。
- 10 分钟 TTL、一次消费。
- 编辑现有账号时 start 请求携带账号 ID，后端从 DB 合并脱敏后缺失的 `client_secret`。
- popup 在点击事件中先同步打开 `about:blank`，再 await start API，避免浏览器 popup blocker。
- 前端 callback message 严格校验：exact origin、`event.source`、provider、nonce。
- callback 页面不含内联可执行脚本；CSP 为 `script-src 'self'`，使用同源外部 `callback.js`。
- JSON payload 中 `<` 被转义，防止提前结束 inert JSON script 元素。

**OAuth flow persistence:** production now uses SQLite `oauth_pending_flows` through `Catalog` and the existing `credentials.key`; state/session are hashes and pending config remains AES-GCM ciphertext. A flow survives a process/container restart and is still one-time consumable. Multi-replica deployment is not supported by the current single-container architecture, but multiple processes sharing the same SQLite file would see the same pending rows.

### 2.8 Endpoint SSRF / DNS rebinding

- `storageproviders.NewEndpointHTTPClient` 在 `DialContext` 内解析、校验 IP，然后直接拨已校验 IP。
- 默认拒绝 loopback、RFC1918/ULA、link-local、unspecified 等地址。
- `ALLOW_PRIVATE_STORAGE_ENDPOINTS=true` 显式放行私网。
- `ALLOW_INSECURE_STORAGE_ENDPOINTS=true` 显式放行 HTTP。
- S3 与 WebDAV 使用该 pinned transport。
- redirect 不自动跟随，避免凭据跨源泄露。

### 2.9 Token 并发更新与删除竞态

`Catalog` 新增：

- `driveCredentialsMu`
- `MergeDriveCredentials(ctx, id, updates)`

行为：

- `UpsertDrive` 与字段级 token merge 共锁。
- OneDrive/Google token callback 重新读取当前行，只 merge `access_token`/`refresh_token`，不覆盖管理员刚更新的 name/root/client secret/其他配置。
- `DeleteDrive` 也获取同一锁，防止 callback 读旧行后管理员删除，再由 callback Upsert 复活网盘。

## 3. 本轮一起修复的工作区既有问题

当前 worktree 本来就包含一批用户未提交的安全/部署改动，本轮为保证全量构建测试顺带修复了其中的编译或测试冲突：

- `streamhttp` imports/安全 client。
- destructive source-delete prepare/confirm 测试语义冲突。
- `migrateConfigAdminUser` / `ensureConfigAdminUser` 兼容 helper。
- 管理员配置明文密码迁移与清理。
- Docker hardening 相关测试。

这些不是全部由本次存储任务新发起；不能在后续会话中 reset/revert 或声称全部归本任务所有。

## 4. 最新验证证据

### Go

执行：

```bash
docker run --rm -v "$PWD/backend:/app" -w /app golang:1.23-bookworm /usr/local/go/bin/go test ./...
docker run --rm -v "$PWD/backend:/app" -w /app golang:1.23-bookworm /usr/local/go/bin/go vet ./...
```

最新结果：

- `go test ./...`：通过，所有 package 绿。
- `go vet ./...`：通过（无输出，组合命令 exit 0）。
- focused：legacy `/admin/api/drives` 对 OneDrive/Google Drive/WebDAV/S3 使用安全兼容 wrapper，转发到统一 provider 的后端 probe-before-save orchestration；旧客户端继续可用且不能绕过 probe。
- race：`internal/storageproviders`、`internal/catalog`、`internal/api`、`internal/scanner`、`cmd/server`、`internal/proxy`、`internal/fingerprint`、`internal/transcode`、`internal/preview` 全部通过。

### Frontend

执行：

```bash
npm run lint
npm test
npm run build
git diff --check
```

最新结果：

- TypeScript lint：通过。
- 前端测试：**242/242 通过**。
- Vite production build：通过，1655 modules transformed。
- `git diff --check`：通过。

### Docker 运行态

- 旧备份 `data/backups/storage-onboarding-20260720-084849/` checksum 仍通过，旧回滚镜像 `hicocos-91:rollback-20260720-084849` 仍存在。
- 本次部署前又创建新一致性备份 `data/backups/storage-onboarding-20260720-010850/`：SQLite online backup、`config.yaml`、`credentials.key`、`.version`、`SHA256SUMS`；checksum 全部通过，SQLite `integrity_check=ok`。
- 本次回滚镜像：`hicocos-91:rollback-20260720-010850`，指向部署前 `sha256:ee823936...`。
- 已执行 `docker compose build video-site-91` 与 `docker compose up -d --force-recreate video-site-91`。
- 当前容器 `video-site-91` 使用镜像 `sha256:caf2bf2d1a7ce3590d4f15f822d8701f9b8b5095120de83008c92f2e76cf7c00`，2026-07-20 02:11:50 UTC 启动，状态 healthy，端口 `127.0.0.1:9191`，用户 `9191:9191`，root filesystem read-only。
- 容器实际环境包含 `VIDEO_PUBLIC_ORIGIN=https://91s.lolicc.cc`、`VIDEO_DATA_DIR=/data`、`VIDEO_CONFIG=/data/config.yaml`。
- `/healthz` 返回 `{"ok":true}`，首页 HTTP 200；SQLite `integrity_check=ok`，`oauth_pending_flows` 表存在且 smoke 后为 0，`drives/videos/users` 行数与备份一致。
- 使用临时 5 分钟管理员 session（验证后已删除）检查：`/admin/api/me` authenticated/admin、provider manifest 共 11 项、S3 `upload=false`、callback JS 200 且为 JavaScript、Google OAuth start 200，解析出的 redirect URI 精确为 `https://91s.lolicc.cc/admin/api/storage/oauth/googledrive/callback`；临时 pending flow 已删除。
- 最近日志未发现 panic/fatal/解密/迁移/SQLite lock/no-such-table；仍有独立的原有 PikPak 验证码 attach 告警。

## 5. 当前工作区与 Git 状态

- 仓库：`/www/91`
- 分支：`main...origin/main [ahead 16]`
- worktree 很脏，包含大量 tracked 修改和 untracked 新文件。
- 本轮没有 commit、没有 push。
- 绝对禁止使用：
  - `git reset --hard`
  - `git checkout -- .`
  - `git clean`
  - 任何会覆盖用户原有修改的恢复操作。

主要新增/修改文件（非完整清单）：

- `backend/internal/storageproviders/{registry.go,security.go,oauth.go,...tests}`
- `backend/internal/drives/s3/{driver.go,driver_test.go}`
- `backend/internal/drives/googledrive/{driver.go,driver_test.go}`
- `backend/internal/drives/onedrive/driver.go`
- `backend/internal/drives/webdav/{driver.go,driver_test.go}`
- `backend/internal/api/admin_storage.go`
- `backend/internal/api/admin_storage_oauth.go`
- `backend/internal/api/admin_storage_test.go`
- `backend/internal/api/admin.go`
- `backend/cmd/server/{drives.go,main.go}`
- `backend/internal/catalog/{catalog.go,drives_test.go}`
- `backend/internal/scanner/{scanner.go,scanner_test.go}`
- `src/admin/{DrivesPage.tsx,api.ts}`
- `src/admin/drive/{DriveForm.tsx,constants.ts}`
- `tests/storageProviderOnboarding.test.ts`
- `docs/superpowers/specs/2026-07-19-storage-provider-onboarding-design.md`
- `docs/superpowers/plans/2026-07-19-storage-provider-onboarding.md`

查看完整状态：

```bash
cd /www/91
git status --short --branch
git diff --stat
git diff --check
```

## 6. 当前卡点 / 未完成事项

### 6.1 最终复审状态

旧 delegation `deleg_5403aea6` 无法恢复，替代性只读复审 `deleg_d695a985` 已返回：Critical 0、Important 4、Minor 2。六项均已逐项核验、TDD 修复并部署：

1. attach 失败 rollback 不再无条件 `UpsertDrive(existing)`；改用基于 candidate `updated_at` 的条件 UPDATE，删除或并发修改后不会复活旧账户。
2. detach 与 attach 共用 lifecycle lock；`SetDriveRuntimeStatus` 对缺行返回 `sql.ErrNoRows`，慢 attach 在删除后不能重新注册 runtime driver/workers。
3. `.strm` 的 `PublicNetworkOnly` 现在贯穿 proxy、fingerprint、transcode 和 preview/ffmpeg loopback proxy，实际拨号使用 public-only pinned client，拒绝私网、混合 DNS 与 redirect rebinding。
4. 旧 `POST /admin/api/drives` 保留兼容，但四类统一 provider 被转发到同一个 storage probe/save orchestration，不再直接 400，也不能绕过 probe-before-save。
5. 编辑表单消费并展示后端 `configured` secret 状态，明确“已配置，留空则保持不变”。
6. S3 删除 `Upload` / `EnsureDir` compatibility stubs，不再满足 optional writable interfaces；manifest 与 crawler target 仍为只读。

验证与部署：

- focused RED/GREEN 覆盖上述竞态、SSRF、兼容 wrapper、configured UI、S3 capability；
- `go test ./...`、`go vet ./...`、重点 package `-race` 全部通过；
- 前端 lint、242/242 tests、production build 通过；
- 新备份 `data/backups/storage-onboarding-review-20260720-021107/` checksum 与 SQLite integrity 通过；
- 回滚镜像 `hicocos-91:rollback-review-20260720-021107`；
- 线上镜像 `sha256:caf2bf2d...` healthy，admin/OAuth smoke 通过。

### 6.2 OAuth flow 已持久化

`backend/internal/storageproviders/oauth.go` 支持可注入持久 store，生产由 `Catalog` 接到 SQLite `oauth_pending_flows`。加密密钥复用 `/data/credentials.key`，数据库不保存 raw state/session 或明文 provider secret。测试覆盖 store 重建后消费与一次性删除。该项不再是部署缺口。

### 6.3 尚无真实第三方账户端到端测试

由于没有用户的 Microsoft/Google/S3/WebDAV 凭据，本轮只完成 mock HTTP、单元/集成测试和构建。上线后必须用真实账户人工验证：

- OneDrive OAuth start → Microsoft → callback → 表单 token 回填 → 测试并保存 → 扫描/播放。
- Google OAuth、个人盘和 Shared Drive。
- Google 文件 shortcut 与 folder shortcut。
- S3 兼容端点 List/Stat/presigned playback/delete，确认无上传。
- WebDAV 添加、原有上传/爬虫迁移不回归。

## 7. 下一会话推荐继续步骤

1. 当前独立复审已闭环，不需要再等待 delegation；除非源码继续变化，不要重复部署。
2. 当前工作区与线上镜像 `sha256:caf2bf2d...` 一致（交接文档本身除外）。
3. 使用真实账户完成尚缺的第三方 smoke：OneDrive/Google OAuth、Google Shared Drive 与 shortcut、S3 List/Stat/play/delete、WebDAV 原有上传。
4. 保留最新备份 `data/backups/storage-onboarding-review-20260720-021107/` 和回滚镜像 `hicocos-91:rollback-review-20260720-021107`，直到真实账户 smoke 完成。
5. 用户没有要求 push；仍不得自动 commit/push。

如后续修改后需要重新部署，继续遵循 SQLite online backup + credentials key 配对、checksum、rollback image、build/recreate/health/admin smoke 的完整流程。

### Git 操作

用户没有要求 push。即使部署成功，也不要自动 push。

如用户之后明确要求提交/推送：

- identity：`hicocos <neko0@foxmail.com>`
- 先分辨并保留 worktree 中用户原有改动。
- 不要把无关修改粗暴混入单个 commit，除非用户明确要求全部提交。

## 8. 本轮踩过的坑

1. **并行子代理修改同一 dirty worktree。**
   - 多次出现子代理在主代理读完后改同一文件，导致补丁覆盖/重复定义/签名来回变化。
   - 后续务必先重新读取再 patch；同一文件不要同时交给多个 agent。

2. **范围误解：把“砍上传”扩大成砍原有上传。**
   - 用户最终要求只是 S3/本次新增上传不要，原有上传保持。
   - 曾错误移除 OneDrive/Google/WebDAV crawler target、拆 `Drive` 接口，后来恢复。

3. **`ProbeStorageAccount` 签名被并行改成两种版本。**
   - 在 `(ctx, drive)` 与 `(ctx, drive, writable)` 之间来回，造成测试编译失败。
   - 当前应以源码实际签名为准；最终设计已经倾向 onboarding probe 纯读。

4. **Docker Go image PATH 异常。**
   - `docker run ... sh -lc 'go test'` 曾出现 `go: not found`。
   - 稳定命令是直接调用 `/usr/local/go/bin/go`。

5. **前端测试中有源码正则合约，不等于真实 UI E2E。**
   - `tests/storageProviderOnboarding.test.ts` 只能证明接线 marker。
   - 真实 popup/OAuth 仍需 Playwright 或线上手测。

6. **浏览器 popup 必须同步打开。**
   - 先 await API 再 `window.open` 会丢失 user activation，被浏览器拦截。
   - 当前已先开 `about:blank` 再导航。

7. **严格 CSP 禁止 inline script。**
   - OAuth callback 最初返回内联 script，审查发现与项目 CSP 方针冲突。
   - 已改成 inert JSON + 同源外部 `callback.js`。

8. **Google shortcut 需要三种 ID 语义。**
   - catalog/删除必须保留 shortcut 自身 ID；
   - 播放需要 target ID；
   - folder shortcut 递归 List 也需要 target parent ID；
   - 同时 scanner 必须有 visited guard 防祖先环。

9. **token callback 不能完整 Upsert 旧快照。**
   - 会覆盖管理员并发修改。
   - 已改字段级 merge，并让 DeleteDrive 共锁防已删除网盘复活。

10. **S3 root 外目录不能映射为空字符串。**
    - 返回空 prefix 会意外列整个 bucket 根或写 bucket 根。
    - 当前改为 error/fail closed。

11. **工具的 orphan recovery。**
    - 某些长 npm 命令返回“effect unknown”，不能据此声称通过。
    - 后续已重新完整执行并取得 242/242 + build exit 0。

## 9. 当前可对用户陈述的准确状态

- 最新 dirty worktree 已构建部署为 `sha256:caf2bf2d...`，容器 healthy；`/healthz`、首页、临时认证 admin API、provider manifest、S3 只读 capability、callback JS 与固定 OAuth redirect 均已实测。
- 最新源码通过：Go `test ./...`、`vet ./...`、重点 package race；前端 lint、242/242 tests、Vite build；Compose config 与 `git diff --check`。
- 最新部署前备份与回滚镜像：`data/backups/storage-onboarding-review-20260720-021107/`、`hicocos-91:rollback-review-20260720-021107`；checksum 和 SQLite integrity 均通过。
- 替代性独立复审 `deleg_d695a985` 的 4 Important / 2 Minor 已全部修复、验证并上线；Critical 0，当前无未闭环复审项。
- 没有 commit、没有 push。
- 没有真实 Microsoft/Google/S3/WebDAV 凭据，因此真实第三方 OAuth 和远端媒体操作仍需管理员手工 smoke test。
- 原有 PikPak 账号仍因上游要求验证码而 attach 失败，这是独立运行告警，不是本次新 provider 部署失败。
