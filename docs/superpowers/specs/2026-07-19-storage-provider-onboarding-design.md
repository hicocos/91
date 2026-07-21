# 存储 Provider 添加与链接链路重构设计

## 目标

重构项目一 `/www/91` 的“添加网盘/链接网盘”流程，深度吸收 `/www/wwwroot/tg-vault` 已验证的账户配置、OAuth、安全校验和连接生命周期模式，同时保持项目一的产品定位：多个网盘可以同时挂载、扫描、播放并作为爬虫上传目标，而不是切换唯一 active storage。

本期完整覆盖 OneDrive、Google Drive、WebDAV，并新增通用 S3 兼容存储（AWS S3、MinIO、Cloudflare R2 等）。其他现有网盘保持行为兼容。

## 范围

### 必须完成

- 后端统一 Provider 注册表，集中管理 kind、显示名称、配置字段、默认根目录、能力、工厂、校验与连接探测。
- 管理后台从后端 Provider manifest 渲染四类重点存储的添加表单，减少前后端重复硬编码。
- 新建和编辑必须先连接探测，探测成功后才持久化；失败不得污染现有配置或替换可用运行实例。
- OneDrive、Google Drive 提供 OAuth 弹窗授权，并保留手动 Refresh Token 兜底。
- OAuth state 必须随机、只存哈希、绑定管理员会话和 provider、配置加密、限时、一次性消费。
- OneDrive token 刷新不再依赖第三方 OpenList renew API；支持个人盘和 SharePoint drive context。
- Google Drive 支持个人盘与 Shared Drive，所有相关请求统一附带团队盘上下文。
- WebDAV 增加只读连接探测、合理传输超时以及 endpoint 安全策略；原有业务上传路径保持兼容。
- 新增 S3 driver：目录列表、Stat、播放和删除；支持 endpoint、region、bucket、AK/SK、可选 session token、force path style、根前缀。S3 在本项目中不新增上传能力。
- OneDrive、Google Drive、WebDAV 原有 Upload/EnsureDir、爬虫上传目标及相关行为保持兼容，不属于本次删除范围。
- S3 object key 是稳定 fileID；ETag 不作为可靠内容 MD5。
- 默认禁止私网 endpoint 和明文 HTTP。仅部署者显式设置 `ALLOW_PRIVATE_STORAGE_ENDPOINTS=true` / `ALLOW_INSECURE_STORAGE_ENDPOINTS=true` 后放行相应范围。
- S3 只做只读连接探测，不创建临时对象，也不作为爬虫上传目标。
- OneDrive、Google Drive、WebDAV 沿用项目原有上传目标与业务上传能力；新 onboarding probe 只做读探测，不创建远端文件。
- 敏感字段编辑时不返回明文；留空表示保留，显式清除使用独立标记。
- token 自动更新使用字段级更新，避免旧内存快照完整覆盖并发编辑。
- 现有 SQLite drive/video 主模型保持兼容，不在本期引入独立 object 表或唯一 active account。

### 不纳入本期

- 一次性迁移所有国内网盘到新实现。
- 把项目一改成项目二的唯一 active storage 模型。
- 普通用户上传页改为任选云盘。
- 对所有历史写链路引入完整持久 reconciliation journal。

## 架构

### 控制面

新增 `backend/internal/storageproviders`：

- `ProviderDescriptor`：kind、manifest、配置规范化、driver 构造、连接探测。
- `Registry`：注册、查找和列出 descriptor。
- `ProviderManifest`：前端可见字段、认证方式、根目录模式、静态能力。
- Endpoint validator：scheme、DNS/IP、重定向目标和私网/HTTP 开关。

四类重点 provider 使用完整 descriptor。其他 provider 通过 legacy descriptor 接入，保持原构造逻辑。

### 数据面

保留现有 `drives.Drive` 的 List/Stat/StreamURL/Upload/EnsureDir/RootID 接口，避免破坏 scanner、proxy、preview、fingerprint、transcode 和 crawlerupload。删除等继续使用 optional capability。

运行时挂载改成：descriptor lookup → build 临时 driver → probe/init → 成功后原子替换 registry/workers。编辑失败时旧实例继续服务。

### API

- `GET /admin/api/storage/providers`：返回 manifest。
- `POST /admin/api/storage/accounts/probe`：测试新账户配置。
- `POST /admin/api/storage/accounts/{id}/probe`：合并已有敏感配置后测试编辑。
- `POST /admin/api/storage/accounts` / `PUT .../{id}`：只接受绑定当前会话、provider、account/revision 和配置摘要的一次性 probe token。
- OAuth start/callback：OneDrive 与 Google Drive 各一组。
- 保留原 `/admin/api/drives` 兼容路径供旧客户端使用，但新 UI 使用 storage accounts API；兼容路径也必须先构建并验证后落库。

## Provider 行为

### OneDrive

- OAuth 或手动 Refresh Token。
- 配置 client ID、可选 client secret、tenant、个人盘或 SharePoint site/drive。
- 服务端直接向 Microsoft token endpoint 刷新。
- Graph base 固定到当前 drive context，所有数据操作一致。
- 上传避免大文件整体读内存，使用 upload session 和专用 HTTP client。

### Google Drive

- OAuth 或手动 Refresh Token。
- Shared Drive 配置持久化。
- Shared Drive 列表使用 `corpora=drive`、`driveId`、`supportsAllDrives`、`includeItemsFromAllDrives`。
- shortcut 保留自身 ID，播放时解析目标，删除不会误删目标文件。

### WebDAV

- 支持匿名或 Basic Auth。
- Root 是 endpoint 下路径。
- PROPFIND 探测读权限；上传目标额外执行随机小文件 PUT/Stat/DELETE 探测。
- metadata 请求有短超时，传输使用 inactivity timeout 与较长总上传上限。
- 不自动跨源跟随重定向，不提供跳过 TLS 验证。

### S3

- AWS endpoint 可留空；兼容端点可填写。
- Root 是规范化 key prefix。
- ListObjectsV2 + Prefix + Delimiter 模拟目录。
- `HeadObject` 做 Stat；presigned GET 做播放；DeleteObject 删除。S3 不提供上传或目录创建。
- OneDrive、Google Drive、WebDAV 原有上传实现不变。
- 不实现 EnsureDir，不创建占位对象。
- 探测只执行 bucket/prefix 读取，不执行临时对象写入。

## 安全与一致性

- OAuth flow 10 分钟过期，一次性消费，绑定管理员 session hash/provider/redirect URI。
- pending OAuth config 使用现有 AES-256-GCM 凭据密钥加密。
- probe token 短时、一次性、绑定配置摘要；不能“测试 A 保存 B”。
- 新建/编辑采用先探测后事务保存；失败不落库。
- API 列表和编辑视图全部脱敏。
- 自动 token 更新仅更新 token 字段并做版本/CAS 保护。
- endpoint 每次实际拨号和重定向都执行地址策略，不能只在保存时做一次 DNS 检查。
- 删除 drive 不删除远端原文件，保持项目一现有语义；有运行任务时继续阻断或先停止。

## 前端体验

- 添加页第一步选择存储类型，第二步显示由 manifest 生成的字段。
- OAuth provider 默认显示“授权连接”，高级区提供手动 Refresh Token。
- 保存按钮语义为“测试并添加”或“测试并保存”。
- 测试期间显示连接步骤；失败保留表单并给可操作错误，不产生 error 账户。
- 编辑敏感字段显示“已配置”，留空不会清除。
- S3/WebDAV 对私网和 HTTP 限制给出明确部署环境变量提示。

## 测试与验收

严格 TDD，至少覆盖：

- Registry/manifest 单一真源和 legacy 兼容。
- probe 成功才保存、失败不写库、编辑失败保留旧实例。
- probe token 的 session/provider/config/revision 绑定、一次消费和过期。
- OAuth state 哈希、session 绑定、并发 flow、一次消费、过期与错误 callback。
- OneDrive OAuth/token refresh、个人盘、SharePoint 和上传。
- Google OAuth、Shared Drive 参数、shortcut 删除语义。
- WebDAV endpoint 安全、超时、读写 probe。
- S3 List/Stat/presign/delete/root boundary/prefix/path-style，并断言 Upload/EnsureDir 零远端请求地返回不支持。
- 前端 manifest 表单、OAuth popup 消息校验、敏感字段编辑和错误保留。
- crawler upload 保留项目原有可写 provider，不把新增只读 S3 显示为上传目标。
- 前后端完整测试、lint/vet/build。
- Docker 镜像重建和容器重建；验证 `127.0.0.1:9191` 健康接口及已部署前端包含 S3/OAuth UI。

## 兼容与发布

- 保留现有 drive IDs、root IDs、video file IDs 与已加密 credentials。
- 旧 OneDrive/Google/WebDAV 配置在加载时规范化并迁移到新 schema，不要求用户重建账户。
- 发布前备份 `/www/91/data`，尤其是 SQLite 与 `credentials.key`。
- 本次不自动提交或推送 GitHub；只有用户明确要求才推送。
