# 91 P0/P1 Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复除首次初始化抢占和爬虫权限以外的已确认 P0/P1 问题，迁移线上 Docker 数据，部署并完成全链路验证。

**Architecture:** 先建立可恢复的数据快照，再用行为测试逐项收紧目录树状态机、视频播放授权、鉴权错误、上传和 HTTP 边界；随后统一 SQLite、容器、Nginx、CI 和发布链配置。所有线上重建都置于备份校验和完整测试之后，旧匿名卷保留作为最终回滚源。

**Tech Stack:** React 18、TypeScript、Vite、Node test、Go、chi、modernc SQLite、Docker Compose、Nginx、GitHub Actions。

## Global Constraints

- 不修改 `/admin/api/setup` 的首次初始化语义。
- 不限制管理员导入或执行爬虫脚本，不增加爬虫 SSRF 拦截或独立沙箱。
- 不推送 GitHub。
- 生产数据路径统一为宿主机 `/www/91/data` → 容器 `/opt/video-site-91/data`。
- 行为修复必须先观察回归测试失败，再写生产代码。
- 在恢复验证前不删除旧匿名卷。
- 不把用户原有的服务器本地 Compose 差异混入无关提交。

---

### Task 1: 生成并校验部署前恢复点

**Files:**
- Create outside repo: `/www/91-backups/<timestamp>/manifest.txt`
- Create outside repo: `/www/91-backups/<timestamp>/video-site.db`
- Create outside repo: `/www/91-backups/<timestamp>/data.tar.gz`
- Create outside repo: `/www/91-backups/<timestamp>/docker-compose.yml`
- Create outside repo: `/www/91-backups/<timestamp>/91s.lolicc.cc.conf`

**Interfaces:**
- Consumes: 当前容器 `video-site-91` 和匿名卷 `e5420ff6...`。
- Produces: 经 `PRAGMA integrity_check` 和 SHA-256 验证的恢复目录；后续 Task 12 只允许在该目录有效时重建。

- [ ] **Step 1: 记录运行态和空间预检**

Run:

```bash
cd /www/91
backup="/www/91-backups/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$backup"
df -B1 /www /var/lib/docker
id=$(docker inspect video-site-91 --format '{{.Id}}')
image=$(docker inspect video-site-91 --format '{{.Image}}')
volume=$(docker inspect video-site-91 | jq -r '.[0].Mounts[] | select(.Destination=="/opt/video-site-91/data") | .Name')
printf 'container=%s\nimage=%s\nvolume=%s\n' "$id" "$image" "$volume" | tee "$backup/manifest.txt"
printf '%s' "$backup" > /tmp/91-backup-path
```

Expected: `/www` 可用空间大于当前数据目录两倍加 1 GiB；volume 非空。

- [ ] **Step 2: 使用 SQLite Online Backup 创建一致性数据库**

Run:

```bash
backup=$(cat /tmp/91-backup-path)
docker exec video-site-91 python3 -c '
import sqlite3
src=sqlite3.connect("/opt/video-site-91/data/video-site.db")
dst=sqlite3.connect("/tmp/video-site-backup.db")
with dst: src.backup(dst)
print(dst.execute("PRAGMA integrity_check").fetchone()[0])
dst.close(); src.close()
'
docker cp video-site-91:/tmp/video-site-backup.db "$backup/video-site.db"
python3 -c 'import sqlite3,sys; c=sqlite3.connect(sys.argv[1]); print(c.execute("PRAGMA integrity_check").fetchone()[0])' "$backup/video-site.db"
```

Expected: 两次均输出 `ok`。

- [ ] **Step 3: 归档完整数据和配置**

Run:

```bash
backup=$(cat /tmp/91-backup-path)
docker exec video-site-91 tar -C /opt/video-site-91 -czf /tmp/video-site-data.tar.gz data
docker cp video-site-91:/tmp/video-site-data.tar.gz "$backup/data.tar.gz"
cp -a /www/91/docker-compose.yml "$backup/docker-compose.yml"
cp -a /www/server/panel/vhost/nginx/91s.lolicc.cc.conf "$backup/91s.lolicc.cc.conf"
sha256sum "$backup"/{video-site.db,data.tar.gz,docker-compose.yml,91s.lolicc.cc.conf} | tee "$backup/SHA256SUMS"
sha256sum -c "$backup/SHA256SUMS"
```

Expected: 四个文件全部 `OK`。

- [ ] **Step 4: 写入迁移前逻辑计数**

Run:

```bash
backup=$(cat /tmp/91-backup-path)
docker exec video-site-91 python3 -c '
import sqlite3,json
c=sqlite3.connect("file:/opt/video-site-91/data/video-site.db?mode=ro",uri=True)
tables=["users","drives","videos","tags","deleted_videos","admin_sessions"]
print(json.dumps({t:c.execute(f"SELECT COUNT(*) FROM {t}").fetchone()[0] for t in tables},sort_keys=True))
' | tee "$backup/db-counts.json"
```

Expected: JSON 可解析并包含全部六个表。

---

### Task 2: 修复目录树失败重试状态机

**Files:**
- Modify: `src/admin/drive/SkipDirsPanel.tsx`
- Create: `src/admin/drive/dirTreeLoadState.ts`
- Test: `tests/adminDriveForm.test.ts`

**Interfaces:**
- Produces: `nextDirTreeLoadState(state, event)` 纯状态转换；组件失败后保持 `error`，仅显式 `retry` 回到 `idle`。

- [ ] **Step 1: 写失败的状态机行为测试**

Add tests that assert:

```ts
assert.equal(nextDirTreeLoadState("idle", "start"), "loading");
assert.equal(nextDirTreeLoadState("loading", "reject"), "error");
assert.equal(nextDirTreeLoadState("error", "effect"), "error");
assert.equal(nextDirTreeLoadState("error", "retry"), "idle");
```

Run:

```bash
npm test -- --test-name-pattern='directory tree load state'
```

Expected: FAIL because module/function does not exist.

- [ ] **Step 2: 实现最小状态机并接入组件**

Create a union state `idle | loading | loaded | error`; use one effect keyed by `open/status/driveId/id`; render a retry button in `error`; use an `active` flag to suppress writes after unmount. Do not include mutable `loading` in a recreated callback dependency loop.

- [ ] **Step 3: 验证专项和前端测试**

Run:

```bash
npm test -- --test-name-pattern='directory tree load state|skip directories'
npm run lint
```

Expected: PASS and no TypeScript errors.

- [ ] **Step 4: Commit**

```bash
git add src/admin/drive/SkipDirsPanel.tsx src/admin/drive/dirTreeLoadState.ts tests/adminDriveForm.test.ts
git commit -m "fix(admin): stop failed directory tree retry loop"
```

---

### Task 3: 将播放入口绑定 Catalog Video ID

**Files:**
- Modify: `backend/internal/api/api.go`
- Modify: `backend/internal/api/api_test.go`
- Modify: `backend/internal/catalog/catalog.go` only if a visibility helper is needed
- Test: `backend/internal/api/api_test.go`

**Interfaces:**
- Produces: `GET /p/stream/{videoID}` and `Server.resolveVisibleStream(ctx, videoID) (driveID, fileID string, error)`。
- Removes public use of `GET /p/stream/{driveID}/*`.

- [ ] **Step 1: 写未入库底层文件不能播放的失败测试**

Construct a catalog video and fake proxy/drive; assert registered route `/p/stream/missing-video` returns 404 without invoking `StreamURL`. Add a visible-video case that resolves only catalog-stored IDs and a hidden-video case returning 404.

Run:

```bash
docker run --rm --mount type=bind,src=/www/91/backend,dst=/src,readonly -w /src -e GOFLAGS=-mod=vendor golang:1.24-bookworm sh -lc 'export PATH=/usr/local/go/bin:$PATH; go test ./internal/api -run Stream -count=1'
```

Expected: FAIL because route still consumes drive/file IDs or helper is missing.

- [ ] **Step 2: 实现按 videoID 解析**

Change generated source URLs to `/p/stream/<videoID>`. Handler loads video, rejects missing/hidden records, verifies drive exists, selects ready transcoded file when present, and only then calls Proxy with database IDs. Local uploads continue through `/p/upload/{videoID}`.

- [ ] **Step 3: 添加旧底层入口和 LocalStorage 负向测试**

Assert `/p/stream/local/<encoded-path>` no longer matches a stream route and returns 404; assert a catalog video whose drive was removed returns 404.

- [ ] **Step 4: 验证专项测试**

Run:

```bash
docker run --rm --mount type=bind,src=/www/91/backend,dst=/src,readonly -w /src -e GOFLAGS=-mod=vendor golang:1.24-bookworm sh -lc 'export PATH=/usr/local/go/bin:$PATH; go test ./internal/api ./internal/proxy -count=1'
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/api/api.go backend/internal/api/api_test.go backend/internal/catalog/catalog.go
git commit -m "fix(stream): authorize playback by catalog video"
```

---

### Task 4: 集中处理会话失效和安全回跳

**Files:**
- Create: `src/admin/authEvents.ts`
- Create: `src/admin/safeReturnPath.ts`
- Modify: `src/admin/api.ts`
- Modify: `src/admin/AuthContext.tsx`
- Modify: `src/admin/RequireAuth.tsx`
- Modify: `src/admin/LoginPage.tsx`
- Test: `tests/adminAuthBehavior.test.ts`

**Interfaces:**
- Produces: `emitUnauthorized()`, `subscribeUnauthorized(listener)`, `safeReturnPath(value, fallback)`。
- Auth status becomes `loading | authed | guest | unavailable`.

- [ ] **Step 1: 写安全回跳和 401 广播失败测试**

Test that `/admin?x=1` is accepted and `//evil.example`, `\\evil`, `https://evil.example` fall back. Mock fetch returning 401 and assert unauthorized listener fires exactly once.

Run:

```bash
npm test -- --test-name-pattern='safe return path|unauthorized event'
```

Expected: FAIL because modules do not exist.

- [ ] **Step 2: 实现纯函数和事件总线**

Use a module-local `Set<() => void>`; `request()` emits on 401 before throwing. `safeReturnPath` accepts strings beginning with exactly one `/` and containing no backslash.

- [ ] **Step 3: 接入 AuthContext 和路由**

Subscribe in `AuthProvider`; 401 sets guest/clears role. Initial `/me` distinguishes successful unauthenticated response from thrown network/5xx and sets `unavailable`; render a retry screen. Apply `safeReturnPath` at both declarative and imperative login redirects.

- [ ] **Step 4: 验证专项与完整前端测试**

Run:

```bash
npm test -- --test-name-pattern='safe return path|unauthorized event|login'
npm run lint
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add src/admin/authEvents.ts src/admin/safeReturnPath.ts src/admin/api.ts src/admin/AuthContext.tsx src/admin/RequireAuth.tsx src/admin/LoginPage.tsx tests/adminAuthBehavior.test.ts
git commit -m "fix(auth): invalidate expired sessions centrally"
```

---

### Task 5: 保留视频详情错误语义并加入路由错误 UI

**Files:**
- Modify: `src/data/videos.ts`
- Modify: `src/pages/VideoDetailPage.tsx`
- Create: `src/components/AppErrorBoundary.tsx`
- Modify: `src/App.tsx`
- Test: `tests/videoDetailErrors.test.ts`
- Test: `tests/navigationUi.test.ts`

**Interfaces:**
- Produces: `HTTPError` carrying `status`; VideoDetail page distinguishes 404/401/transient.

- [ ] **Step 1: 写失败测试**

Mock 404, 401 and 500; assert detail fetch rejects with status instead of returning null. Add source-level/behavior test that Suspense fallback is visible and top-level error boundary exists.

- [ ] **Step 2: 实现请求错误类型和页面状态**

Do not catch all detail errors. Handle 404 as not found, 401 via auth flow, and transient errors with retry button. Replace `fallback={null}` with an accessible loading indicator and wrap routes with Error Boundary.

- [ ] **Step 3: 验证并提交**

Run:

```bash
npm test -- --test-name-pattern='video detail error|navigation'
npm run lint
```

Commit:

```bash
git add src/data/videos.ts src/pages/VideoDetailPage.tsx src/components/AppErrorBoundary.tsx src/App.tsx tests/videoDetailErrors.test.ts tests/navigationUi.test.ts
git commit -m "fix(ui): preserve API error semantics"
```

---

### Task 6: 上传硬限制、磁盘预检和并发门禁

**Files:**
- Modify: `backend/internal/config/config.go`
- Modify: `backend/config.example.yaml`
- Modify: `backend/internal/api/api.go`
- Modify: `backend/internal/api/api_test.go`

**Interfaces:**
- Adds config `server.max_upload_bytes` default `10737418240` (10 GiB) and `server.upload_reserve_bytes` default `1073741824` (1 GiB).
- `api.Server` receives upload limits and a semaphore of size 1.

- [ ] **Step 1: 写超限和并发失败测试**

Assert a body exceeding a small injected limit returns 413 and creates no file/catalog record; insufficient available space returns 507; a second concurrent upload returns 429.

- [ ] **Step 2: 实现 MaxBytesReader 和空间检查**

Wrap body before multipart parsing; map `MaxBytesError` to 413; use filesystem free bytes minus declared/reserved size; acquire nonblocking semaphore; always `RemoveAll()` and delete partial destination on errors.

- [ ] **Step 3: 验证并提交**

Run:

```bash
docker run --rm --mount type=bind,src=/www/91/backend,dst=/src,readonly -w /src -e GOFLAGS=-mod=vendor golang:1.24-bookworm sh -lc 'export PATH=/usr/local/go/bin:$PATH; go test ./internal/api ./internal/config -run Upload -count=1'
```

Commit:

```bash
git add backend/internal/config/config.go backend/config.example.yaml backend/internal/api/api.go backend/internal/api/api_test.go
git commit -m "fix(upload): enforce size and capacity limits"
```

---

### Task 7: HTTP、Cookie、SQLite 和健康检查

**Files:**
- Modify: `backend/internal/auth/auth.go`
- Modify: `backend/internal/auth/auth_test.go`
- Modify: `backend/internal/catalog/catalog.go`
- Modify: `backend/internal/catalog/schema.sql`
- Modify: `backend/cmd/server/main.go`
- Modify: `backend/cmd/server/main_test.go`
- Modify: `backend/cmd/server/http.go`
- Modify: `backend/cmd/server/cors_test.go`

**Interfaces:**
- Adds `GET /healthz` returning 200 JSON only when SQLite `SELECT 1` succeeds.
- Session storage hashes tokens with SHA-256; existing sessions may be invalidated during deploy.
- Cookie `Secure` follows trusted forwarded HTTPS or explicit production setting.

- [ ] **Step 1: 写失败测试**

Tests: health returns 200 for open DB and 503 for closed/unavailable DB; cookie is Secure for forwarded HTTPS; session table does not contain raw token; expired session cleanup deletes rows; HTTP server factory exposes nonzero `ReadHeaderTimeout`, `IdleTimeout`, `MaxHeaderBytes`.

- [ ] **Step 2: 实现最小加固**

Add Catalog `Ping`, hashed session key helper and cleanup query. Set DB connection pool to a conservative small writer-compatible size. `chmod 0600` DB after open. Add server timeouts without short write timeout. Mount `/healthz` outside auth.

- [ ] **Step 3: 验证并提交**

Run:

```bash
docker run --rm --mount type=bind,src=/www/91/backend,dst=/src,readonly -w /src -e GOFLAGS=-mod=vendor golang:1.24-bookworm sh -lc 'export PATH=/usr/local/go/bin:$PATH; go test ./internal/auth ./internal/catalog ./cmd/server -count=1'
```

Commit:

```bash
git add backend/internal/auth backend/internal/catalog backend/cmd/server
git commit -m "fix(server): harden sessions HTTP and health checks"
```

---

### Task 8: 统一 Go 和前端依赖版本

**Files:**
- Modify: `backend/internal/drives/scriptcrawler/crawler_test.go`
- Modify: `package.json`
- Modify: `package-lock.json`
- Modify: `Dockerfile` only if Go version decision changes
- Modify: `.github/workflows/release.yml`

**Interfaces:**
- Keep Go 1.23 compatibility by replacing `t.Chdir` with `os.Chdir` plus cleanup.
- Upgrade `react-router-dom` to `6.30.4` or later compatible 6.x; Vite to at least `8.0.16`; tsx/esbuild to a lockfile combination without current advisories.

- [ ] **Step 1: 证明 Go 1.23 当前失败**

Run Go 1.23 full suite and capture the `t.Chdir undefined` failure.

- [ ] **Step 2: 修改测试兼容 Go 1.23 并验证**

Use `old, _ := os.Getwd(); os.Chdir(tmp); t.Cleanup(func(){ os.Chdir(old) })`.

- [ ] **Step 3: 升级前端锁文件并审计**

Run:

```bash
npm install --save-exact react-router-dom@6.30.4
npm install --save-dev vite@8.0.16 tsx@latest
npm audit --omit=dev
npm audit
```

Expected: production audit has zero vulnerabilities; any remaining dev-only advisory must be documented or upgraded.

- [ ] **Step 4: 完整验证并提交**

Run Node 20 lint/test/build and Go 1.23 test/vet.

Commit:

```bash
git add backend/internal/drives/scriptcrawler/crawler_test.go package.json package-lock.json Dockerfile .github/workflows/release.yml
git commit -m "chore: align supported toolchains"
```

---

### Task 9: Docker Compose、镜像降权和数据路径

**Files:**
- Modify: `Dockerfile`
- Modify: `docker-entrypoint.sh`
- Modify: `docker-compose.yml`
- Test: `tests/indexReferrerPolicy.test.ts` or a new `tests/dockerHardening.test.ts`

**Interfaces:**
- Runtime UID/GID fixed to 9191.
- Writable paths: `/opt/video-site-91/data`, `/tmp` only.
- Host binding: `127.0.0.1:9191:9191`.

- [ ] **Step 1: 写 Compose/Dockerfile 结构失败测试**

Read Dockerfile/Compose and assert USER exists, correct mount target, healthcheck, loopback binding, read_only, tmpfs, cap_drop, no-new-privileges, init, resource/PID/log limits.

- [ ] **Step 2: 修改镜像和 Entrypoint**

Create user/group during image build; pre-create and chown app/data directories; entrypoint safely handles mounted data ownership only when allowed, then drops to fixed user (or runs directly as fixed user after host migration ownership is prepared).

- [ ] **Step 3: 修改 Compose**

Set build arg VERSION from environment, correct mount, loopback port, healthcheck `/healthz`, `read_only`, `/tmp` tmpfs, `cap_drop: ALL`, `security_opt: no-new-privileges:true`, `init:true`, memory/CPU/PID and json-file rotation.

- [ ] **Step 4: Render/build test and commit**

Run `docker compose config --quiet` and build a test image without replacing live container.

Commit only intended repository files; retain local deployment-specific image/repo values.

---

### Task 10: Nginx 安全配置

**Files:**
- Backup already created in Task 1
- Modify live: `/www/server/panel/vhost/nginx/91s.lolicc.cc.conf`
- Modify: `index.html` to move inline theme bootstrap if required
- Create: `public/theme-bootstrap.js` if externalized
- Test: `tests/indexReferrerPolicy.test.ts`

**Interfaces:**
- TLS only 1.2/1.3.
- XFF is exactly `$remote_addr`.
- Headers include HSTS, nosniff, frame restriction/CSP and referrer policy.

- [ ] **Step 1: 写静态资源/CSP 兼容失败测试**

Assert theme bootstrap is external and no inline executable script remains if CSP uses `script-src 'self'`.

- [ ] **Step 2: 外置主题脚本并验证前端**

Move pre-mount localStorage theme code to `/theme-bootstrap.js` loaded synchronously.

- [ ] **Step 3: 定向修改 Nginx 并语法检查**

Set `ssl_protocols TLSv1.2 TLSv1.3`; modern cipher list; `X-Forwarded-For $remote_addr`; add headers. Run `/www/server/nginx/sbin/nginx -t` before reload.

- [ ] **Step 4: Reload and external probe**

Reload Nginx without restarting app. Curl HTTP/HTTPS and inspect headers/protocols.

---

### Task 11: CI、发布校验和与不可变 Release

**Files:**
- Create: `.github/workflows/ci.yml`
- Modify: `.github/workflows/docker-build.yml`
- Modify: `.github/workflows/release.yml`
- Modify: `scripts/build-release.sh`
- Modify: `install.sh`
- Create: `.github/dependabot.yml`
- Test: `tests/installScript.test.ts`

**Interfaces:**
- Release assets include `SHA256SUMS`.
- Installer downloads and verifies matching checksum before extraction.
- Existing release tag is never deleted/overwritten.

- [ ] **Step 1: 写安装器校验和失败测试**

Assert install script fetches `SHA256SUMS`, calls `sha256sum -c`, and fails closed on mismatch; assert release workflow lacks `gh release delete`.

- [ ] **Step 2: 实现 CI gate**

CI jobs run Node 20 `npm ci`, lint, tests, build, audits; Go 1.23 test/vet/govulncheck; Docker build plus Trivy and SBOM. Add timeout, concurrency and minimal permissions.

- [ ] **Step 3: 实现不可变发布和摘要验证**

Build script writes SHA256SUMS. Release creates once and fails if tag release already exists. Installer downloads checksum and verifies selected asset. Pin self-update to release ref instead of mutable main when VERSION is known.

- [ ] **Step 4: 本地静态验证和提交**

Run shell syntax, YAML parse, install tests and build-release dry prerequisites without publishing.

---

### Task 12: 迁移宿主数据并部署新容器

**Files:**
- Runtime data: `/www/91/data`
- Runtime Compose: `/www/91/docker-compose.yml`

**Interfaces:**
- Requires Task 1 recovery point and Tasks 2–11 verification green.
- Produces healthy live container using host bind data.

- [ ] **Step 1: 停止写入并复制最终数据**

Stop only `video-site-91`; extract Task 1 archive into a staging directory; replace its database with the online backup or take a final stopped copy including WAL checkpoint; verify integrity and counts; atomically move staging to `/www/91/data`; chown UID/GID 9191 and chmod config/DB 0600.

- [ ] **Step 2: Validate Compose and build versioned image**

Set `VERSION=$(git describe --tags --always --dirty)` for build; run compose config; build without cache only if needed.

- [ ] **Step 3: Recreate app service**

Run `docker compose up -d --build --force-recreate video-site-91`; wait for healthy.

- [ ] **Step 4: Verify mount and data parity**

Inspect live mounts; ensure no anonymous volume at data destination; run SQLite integrity and compare counts to Task 1; verify config setup remains false.

- [ ] **Step 5: Roll back immediately on any mismatch**

Restore backed-up Compose/Nginx, reattach old anonymous volume or restore backup data, start old image and re-run health/login probes.

---

### Task 13: 完整回归、运行态验证和收尾

**Files:**
- No new production files unless a test reveals a defect.

**Interfaces:**
- Produces final evidence ledger and leaves no GitHub push.

- [ ] **Step 1: Full frontend verification**

Run Node 20 `npm ci`, lint, 215+ tests, build and audits.

- [ ] **Step 2: Full backend verification**

Run Go 1.23 `go test ./... -count=1`, `go vet ./...`, and focused race tests for auth/catalog/api if runtime permits.

- [ ] **Step 3: Container hardening verification**

Inspect UID, mounts, health, readonly rootfs, caps, no-new-privileges, limits, PID/log options; prove writes fail outside data/tmp and succeed inside data/tmp.

- [ ] **Step 4: Functional probes**

Verify `/`, `/healthz`, `/admin/api/setup`, unauthorized API 401, old raw stream path 404, catalog stream path authorization, Range response, upload 413, directory retry behavior, login/session expiration behavior.

- [ ] **Step 5: Nginx/HTTPS probes**

Verify HTTP→HTTPS, TLS 1.2/1.3 only, security headers, login page loads and browser console has no errors.

- [ ] **Step 6: Log regression check**

Observe logs for a bounded period; aggregate status codes and prove no repeated dirtree 500 storm.

- [ ] **Step 7: Git and delivery check**

Confirm exact commits/diff, no secrets, no accidental backup files, no remote push. Report backup path, retained old volume, deployment image ID, test counts and any intentionally deferred risks.
