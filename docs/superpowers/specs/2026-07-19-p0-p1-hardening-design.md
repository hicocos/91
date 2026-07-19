# 91 P0/P1 修复与部署加固设计

日期：2026-07-19  
审查基线：`main@b2ccd7436b6538e81c6bd4ce233bdbfae9521457`

## 目标

在不处理首次部署初始化抢占风险、也不限制爬虫脚本导入/执行能力的前提下，修复审查确认的其余 P0/P1 问题，迁移当前线上 Docker 数据，完成构建、部署和验证，但不推送 GitHub。

## 明确排除

- 不修改 `/admin/api/setup` 的首次初始化语义。
- 不限制管理员导入或执行爬虫脚本，不为爬虫增加 SSRF 拦截或独立沙箱。
- 不新增与本轮风险治理无关的业务功能或大规模视觉改版。

## 实施原则

- 先备份和验证数据，再修改会导致容器重建的配置。
- 行为修复严格执行测试先行：先观察回归测试失败，再写最小实现。
- 将源码、部署配置、运行态和外部 HTTPS 入口作为一个整体验证。
- 保留可回滚的旧匿名卷、源码备份和数据库快照，完成恢复验证前不清理。
- 不提交或推送用户原有的服务器本地 Compose 差异；最终提交范围必须可审计。

## 1. 数据持久化与升级回滚

当前应用实际写入 `/opt/video-site-91/data`，但 Compose 将宿主机 `./data` 挂载到 `/www/91/data`，导致 Docker 在真实数据路径创建匿名卷。修复流程：

1. 记录容器、镜像、匿名卷和数据库状态。
2. 使用 SQLite 在线备份生成一致性数据库快照，并执行 `integrity_check`。
3. 归档匿名卷中的配置、预览、上传和爬虫数据。
4. 将完整数据复制到宿主机 `/www/91/data`，保留权限和时间戳。
5. 将 Compose 挂载目标改为 `/opt/video-site-91/data`。
6. 重建后验证 live mount source、配置、用户、网盘和数据库计数。
7. 旧匿名卷在最终验证后仍保留，不自动删除。

安装器升级前也应创建 SQLite 一致性快照，而不仅备份二进制和配置。回滚同时恢复旧程序与匹配的数据库快照。

## 2. 后台目录树重试

`SkipDirsPanel` 根节点默认展开；请求失败后 `loaded=false`，而 callback 又依赖 `loading`，造成 effect 连续重跑。

新状态机：

- 初始：`idle`
- 用户展开或根节点首次挂载：`loading`
- 成功：`loaded`
- 失败：`error`
- `error` 不自动重试，只显示错误和“重试”按钮。
- 同一节点只允许一个在途请求；卸载后不写回状态。

验证包括失败只请求一次、点击重试才产生第二次请求、成功后不重复请求。

## 3. 播放资源授权

浏览器不再直接决定底层 `driveID/fileID`。受控播放地址改为按视频 ID：

- 原视频：`GET /p/stream/{videoID}`
- 转码产物：由服务端根据 catalog 中 `transcode_status/transcoded_file_id` 选择，不接受任意底层 ID。

服务端在调用 Proxy 前必须：

1. 查询 Catalog 中的视频。
2. 确认记录存在且可见、未被墓碑屏蔽。
3. 确认所属 Drive 仍存在（本地上传走其专用路径）。
4. 只使用数据库中的 `drive_id/file_id` 或已就绪的转码产物 ID。

旧 `/p/stream/{driveID}/*` 入口不再暴露任意底层文件访问。刷新页面后前端会收到新地址。测试覆盖未入库、隐藏、墓碑、任意 LocalStorage 文件和转码选择。

## 4. 鉴权和 API 错误语义

- 后台统一请求器收到 401 时广播 session invalidation，`AuthContext` 切换为 `guest`，路由守卫安全回跳登录。
- 登录回跳地址只允许单 `/` 开头的站内路径；拒绝 `//`、反斜杠和外部 URL。
- 鉴权初始化区分 `guest` 与 `unavailable`；网络/502 不伪装成未登录。
- 视频详情区分 401、404、网络故障和 5xx，不再把所有错误吞成 `null`。
- HTTPS 生产部署设置 `Secure` Cookie；本地 HTTP 开发保持可用。
- Nginx 真实 IP 头改为覆盖式 `$remote_addr`，与后端信任边界一致。

## 5. HTTP、上传和 SQLite 加固

- 上传总大小使用 `http.MaxBytesReader` 硬限制，限制值配置化并返回 413。
- 写入前检查数据卷可用空间，限制并发上传；失败删除 multipart 临时文件和目标文件。
- 可在入库前使用 ffprobe 验证媒体，但不扩大本轮为转码或内容审核功能。
- HTTP Server 增加 `ReadHeaderTimeout`、`IdleTimeout`、`MaxHeaderBytes`；流媒体响应不设置会截断长播放的短 `WriteTimeout`。
- SQLite 保留 WAL/busy timeout，并设置适合单实例的连接池上限；数据库文件权限收紧为 0600。
- 启动或定时清理过期 session；新 session 只在数据库保存 token 哈希，兼容迁移现有 session 时允许用户重新登录。
- 新增 `/healthz`，至少验证进程和 SQLite 可查询，不依赖外部网盘健康。

## 6. 容器和 Nginx

Docker：

- 专用非 root UID/GID。
- 仅 `/opt/video-site-91/data` 和 `/tmp` 可写。
- `read_only: true`、`tmpfs: /tmp`、`cap_drop: [ALL]`、`no-new-privileges:true`。
- `init:true`、healthcheck、合理的 CPU/内存/PID 和日志轮转限制。
- 端口只发布到 `127.0.0.1:9191`，由本机 Nginx 对外提供 HTTPS。
- 本地构建注入 Git SHA/版本，不再显示笼统 `dev`。

Nginx：

- 移除 TLS 1.1 和 3DES。
- `X-Forwarded-For` 使用 `$remote_addr`，避免客户端伪造链进入后端判断。
- 增加 CSP、`X-Content-Type-Options`、`frame-ancestors`、合适的 Referrer Policy。
- CSP 兼容现有 Google Fonts 和主题内联脚本：优先把主题脚本移到静态模块，随后收紧 `script-src`。

## 7. 工具链和供应链

- 统一项目实际测试与构建所需 Go 版本；若保留 Go 1.23，则移除测试中的 Go 1.24 API；否则统一升级 Docker、go.mod、Workflow 和文档。
- 升级 React Router 至修复版本，并将 Vite/esbuild 更新到无当前告警版本。
- CI 门禁包含：前端安装、typecheck、unit test、build、npm audit；后端 test、vet、govulncheck；容器构建、Trivy/Grype、SBOM。
- GitHub Actions 固定到可信 SHA，并设置最小权限、timeout、concurrency。
- Release 发布校验和与 attestations，不删除并覆盖已有同 tag Release。
- 安装器验证下载制品摘要；自更新绑定明确版本而不是未经验证地执行 `main`。

## 8. 部署与验证

部署前门禁：

- 所有新增回归测试已经历 RED→GREEN。
- 完整前端 lint/test/build 通过。
- 完整 Go test/vet 通过。
- Compose 渲染通过，数据备份和恢复检查通过。

部署后验证：

- 容器健康、非 root、只读根文件系统和资源限制生效。
- `/opt/video-site-91/data` 的 live mount source 是 `/www/91/data`。
- SQLite `integrity_check=ok`，用户/网盘/视频计数与迁移前一致。
- 登录页、登录态、401 回登录、视频列表/详情、播放 Range、上传限制、目录树错误重试均符合预期。
- Nginx HTTPS 正常，HTTP 跳转正常，安全头和真实 IP 配置生效。
- 日志中不再出现目录树 500 风暴。

## 回滚

如任一部署后检查失败：

1. 停止新容器。
2. 恢复部署前 Compose/Nginx 备份。
3. 重新挂载保留的旧匿名卷，或从一致性快照恢复宿主数据目录。
4. 启动旧镜像并验证 SQLite、登录和页面。
5. 保留失败现场日志，不清理任何卷或备份。

## 成功标准

- 无数据丢失，持久化路径明确且可备份。
- 目录树失败不会自动风暴重试。
- 登录用户不能通过构造底层 Drive/File ID 读取 Catalog 外文件。
- 上传、HTTP、会话和数据库具备明确资源边界。
- 生产容器降权且健康检查可用。
- 完整测试、构建、运行态和 HTTPS 回归通过。
- 没有向 GitHub 推送。