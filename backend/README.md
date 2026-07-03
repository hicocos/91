# backend

视频聚合站的 Go 后端。提供三件事：

1. 多家网盘统一抽象（夸克 / 115 / PikPak / 联通网盘 / 光鸭网盘 / OneDrive / Google Drive / 本地存储）
2. 视频元数据目录（SQLite）+ 扫描 + 预览视频预生成
3. REST API（前台）+ 管理后台 + 直链代理
4. 标签池、视频隐藏、按网盘统计和详情页来源网盘类型展示能力

## 目录

```
cmd/server/main.go          入口
internal/
  config/                   YAML 配置
  catalog/                  SQLite 元数据
  drives/
    iface.go                Drive 接口
    quark/                  夸克（自己实现，参考 OpenList quark_uc）
    p115/                   115（壳子 + SheltonZhu/115driver）
    pikpak/                 PikPak（自己实现，参考 OpenList pikpak）
    wopan/                  联通网盘（壳子 + OpenListTeam/wopan-sdk-go）
    guangyapan/             光鸭网盘（参考 AList GuangYaPan）
    onedrive/               OneDrive（OpenList 在线续期 + Microsoft Graph 文件接口）
    googledrive/            Google Drive（OpenList 在线续期 + Google Drive API；播放走后端代理）
    localstorage/           本地目录扫描（服务器已有视频目录）
  scanner/                  扫目录 → 落库
  preview/                  ffmpeg 抽封面和生成多段预览视频
  proxy/                    /p/stream/*、/p/preview/* 代理
  auth/                     管理员 session
  api/                      REST 路由
config.example.yaml         配置模板
```

## 开发环境（Windows）

本仓库假设工具都装在用户目录，不需要管理员权限。

```
C:\Users\<you>\tools\
  go\bin\go.exe             Go 1.23+
  ffmpeg\bin\ffmpeg.exe     任意 ≥ 4.x 版本
```

并加到 `PATH`。

### 第一次启动

Git Bash / WSL 环境推荐从仓库根目录启动完整开发环境：

```bash
npm install
./start.sh               # 默认前端 production preview，无热更新
```

需要前端开发热更新时再用 `FRONTEND_MODE=dev ./start.sh --restart`。

PowerShell 下可以分两个终端手动启动，后端命令如下：

```powershell
cd F:\VideoProject\backend
go run ./cmd/server
```

首次启动会在当前目录创建：

- `config.yaml`（从 `config.example.yaml` 复制）
- `data/video-site.db`
- `data/previews/`

默认监听 `127.0.0.1:9192`。首次部署如果仍是默认管理员配置，登录页会要求先设置用户名和密码，并写回 `config.yaml`。如果本地已有旧的 `config.yaml`，请确认 `server.listen` 与前端代理端口一致。

### 连接前端

`vite.config.ts` 已经把 `/api`、`/p`、`/admin/api` 代理到 `127.0.0.1:9192`。
代理会转发真实来源 IP，登录失败封禁会按真实客户端 IP 计数。

```
npm run build       构建前端静态资源
npm run preview     前端 9191，无热更新
go run ./cmd/server 后端 9192
```

### 真实 IP 与登录封禁

登录失败 3 次后会永久封禁来源 IP，只能在后台用户管理里手动解除。后端默认只信任来自本机代理（`127.0.0.1` / `::1`）的 `X-Forwarded-For` / `X-Real-IP`，外部直连请求伪造这些头会被忽略。

如果前面套 nginx，反代配置必须覆盖真实 IP 头，避免把所有用户都算成 `127.0.0.1`：

```nginx
location / {
    proxy_pass http://127.0.0.1:9191;

    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $remote_addr;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

单机 nginx 反代不要使用 `$proxy_add_x_forwarded_for`，否则客户端自带的伪造 `X-Forwarded-For` 可能进入转发链。

## 添加一个盘

推荐在前端管理后台 `/admin/drives` 新增网盘。保存后会立即挂载并触发扫描；视频结果可在 `/admin/videos` 按网盘查看，每页 100 条，页面会同时显示各网盘预览视频已生成、待生成、失败数量。

也可以直接调用后端接口：

1. 先在浏览器访问 `/login` 完成首次管理员设置，或使用已有管理员账号登录：`POST /admin/api/login`
2. 新建盘：`POST /admin/api/drives`
   ```json
   {
     "id":   "my-quark",
     "kind": "quark",
     "name": "我的夸克盘",
     "rootId": "0",
     "credentials": {
       "cookie": "粘贴浏览器 F12 复制的 pan.quark.cn Cookie"
     }
   }
   ```
3. 手动触发扫描：`POST /admin/api/drives/my-quark/rescan`

各网盘的凭证字段：

| kind   | credentials 字段                                              |
|--------|---------------------------------------------------------------|
| quark  | `cookie`                                                      |
| p115   | `cookie`（形如 `UID=...; CID=...; SEID=...; KID=...`）         |
| pikpak | `username`、`password`（token、验证码和设备 ID 由服务端自动处理并保存） |
| wopan  | `access_token`、`refresh_token`，可选 `family_id`              |
| guangyapan | 推荐后台扫码登录自动写入 `access_token`、`refresh_token`；也可手工填写 token；可选 `root_path` |
| onedrive | `refresh_token` |
| googledrive | 默认只需 `refresh_token`；自建 OAuth 客户端模式还需 `use_online_api=false`、`client_id`、`client_secret` |
| localstorage | `path`（服务器上的已有视频目录，如 `/mnt/videos`） |

### PikPak 速度说明

`disable_media_link` 默认按 `true` 处理，会使用 PikPak 的 `web_content_link` 原始下载链接；在当前服务器实测，单连接通常只有约 2.8-3 MiB/s。把该字段设置为 `false` 后，驱动会请求 `usage=CACHE` 并优先使用 `medias[].link.url`，当前服务器实测 `/p/stream` 64 MiB Range 可到约 8.9 MiB/s。

当前服务器同时存在 sing-box TUN 透明代理，PikPak 默认出站会被 `tun0` 接管；但强制直连物理网卡并没有更快，慢速的主要差异来自 PikPak 取链方式。media/cache CDN 节点仍有波动，偶尔可能遇到慢节点；如果播放变慢，可重新获取直链或重新挂载 PikPak 后再测。

OneDrive 按 OpenList 默认应用方式调用 `https://api.oplist.org/onedrive/renewapi` 在线刷新 token，不需要配置 Azure 应用的 `client_id` / `client_secret` / `redirect_uri`。后台新建 OneDrive 时只需要填 OpenList 代刷得到的 `refresh_token`；服务端会默认挂载根目录并自动回写新 token。

Google Drive 默认按 OpenList 在线 API 调用 `https://api.oplist.org/googleui/renewapi` 刷新 token。后台新建 Google Drive 时只需要填 OpenList Google Drive 获取到的 `refresh_token`。如果不想依赖 OpenList 在线 API，可以关闭“使用 OpenList 在线续期 API”，并填写同一个 Google OAuth 客户端授权得到的 `refresh_token`、`client_id`、`client_secret`，服务端会直接请求 Google OAuth token 接口续期。Google Drive 下载地址必须携带 `Authorization` 头，浏览器不能直接 302 使用，所以本站会由后端代理 `/p/stream` 播放，不加入零带宽 302 白名单。

## 文件名约定

扫描器按以下顺序解析文件名，用于提取标题和作者：

1. `[前缀] 标题 - 作者.mp4`
2. `[前缀] 标题.mp4`
3. `标题 - 作者.mp4`
4. `标题.mp4`

开头的 `[前缀]` 只会从标题里剥离，不会按分隔符作为任意标签入库。视频标签来自三类规则：

1. 标题、文件名、作者和目录名命中标签规则。
2. 脚本爬虫返回的标签只会挂载站内已经存在的标签；爬虫名称会作为独立来源标签。
3. 番号会归并到自动生成标签 `AV`；同一番号前缀至少出现 3 次时，还会生成系列标签，例如 `ABP-123`、`ABP-456` 归入 `ABP`。

不会根据目录名自动创建合集标签，也不会把每个番号建成独立标签。

## 标签系统

标签匹配由统一规则引擎完成。每个标签可以在 `/admin/tags` 编辑以下规则：

- 包含词：在字段中做子串匹配。
- 整词：只匹配完整的字母/数字段或 CJK 单字。
- 排除词：先排除容易误判的文本区域，再执行匹配。
- 番号识别：`AV` 自动生成标签可识别常见番号格式。

单个 CJK 字符和不超过 3 个字符的纯 ASCII 短词会强制使用整词匹配，避免 `臀`、`3P` 等短词误伤。未配置显式规则的自定义标签会使用“标签名 + 别名”生成兼容规则。

标签来源分层维护：

- `builtin`：程序提供的 7 个内置标签：`奶子`、`女大`、`人妻`、`后入`、`制服`、`美臀`、`口交`。历史内置标签不在这个白名单内的会降级为 `generated`；旧 `臀` 会迁移合并为 `美臀`。
- `user`：管理员创建的自定义标签。
- `generated`：程序自动生成的标签，例如番号系列、爬虫名称和历史数据迁移得到的标签。
- 视频关联来源还包括 `auto`、`manual`、`crawler`、`series` 和 `propagated`，后台视频列表会展示来源和匹配依据。

这里的三类是标签本身的类型；视频关联来源记录的是标签如何挂到视频上，两者用途不同。所有三类标签都允许管理员删除。删除会同步移除全部视频关联并留下墓碑；自动流程在升级、重启和后台维护时都不会重建已删除的同名标签。管理员主动新建同名标签时才会解除墓碑，并作为自定义标签重新加入。

手动保存视频标签后，该视频会被整体标记为人工锁定（`tags_manual=1`）；后续扫描、全库重算、系列同步和传播都不会修改它。爬虫名标签是来源标签，会按爬虫视频归属单独维护。

凌晨流水线在视频去重后执行标签维护：

1. 对上次运行后新增或改名的视频增量重打。
2. 同步达到阈值的番号系列标签。
3. 清除上一轮传播结果，再按相同文件指纹和标题聚类重新传播高频标签。
4. 回收零引用的自动生成标签。

管理后台可以手动启动“重新整理所有标签”。任务会先清理普通自动生成标签、`auto/legacy/series` 关联和标签墓碑，并恢复内置标签；如果“自动生成标签”开启，会继续分批重建自动匹配并同步番号系列标签，关闭时只执行清理和爬虫等非自动整理。自定义、内置和爬虫脚本等非普通自动生成标签会保留。升级到新标签系统时也会在后台自动执行一次，不阻塞服务启动。

服务重启时只同步执行数据库结构迁移和少量内置标签种子。标签系统的一次性升级尚未完成时，旧标签关系回填、番号标签归并、存量缺失标签补全和孤儿旧标签清理会在 HTTP 端口开始监听后进入后台单实例任务；升级完成后的正常重启不会触发标签维护。

## 视频去重

项目有三层去重：

1. 同一网盘同一文件按 `(drive_id, file_id)` 形成稳定视频 ID，重复扫描只更新同一行。
2. 扫描时优先按网盘侧 `content_hash` 去重；没有 hash 时退化为 `file_name + size_bytes`。
3. 扫描、本地上传或服务启动挂载网盘后，后台指纹 worker 会异步读取视频的少量 Range 片段，生成 `sampled_sha256`。前台列表、首页、搜索、推荐会按 `size_bytes + sampled_sha256` 只展示最早入库的 canonical 视频。

`sampled_sha256` 是文件级去重：适合识别同一个视频文件被复制到 115 / PikPak / OneDrive / Google Drive 等不同网盘的情况。它不会删除任何网盘文件，也不用于识别转码、裁剪、加水印后的同源视频。

封面和预览视频仍然优先生成，不等待指纹完成。夜间流水线最后会做一次重复资产清理：对 `size_bytes + sampled_sha256` 命中的非 canonical 视频，只删除本机生成的重复封面和预览视频，并把对应字段重置为 `pending`。网盘原文件和视频元数据记录不会被删除；如果 canonical 视频以后被移除，这些重复项会重新进入生成队列。

## 管理能力

- `/admin/drives`：新增、编辑、删除网盘，触发扫描。
- `/admin/videos`：按网盘筛选视频，每页 100 条分页，查看各网盘预览视频统计，编辑标题/作者/分类/标签，单条或全量重生预览视频；拉黑视频页可查看被删除或被隐藏且源文件仍存在的视频。普通网盘解除墓碑后等待下次手动或定时扫盘，爬虫来源会同时清理 seen 记录并等待下次爬虫任务；操作本身不会立即触发重任务。也可以启动单实例后台任务，顺序删除全部黑名单源文件；成功项会删除源文件并清除黑名单记录，失败项保留待重试；爬虫来源会继续保留已爬取 source_id，避免后续重复爬取。
- `/admin/tags`：新增、编辑标签规则并增量匹配已有视频；所有标签均可删除，删除时会从所有视频同步移除；可以手动启动全库标签整理，并查看后台任务进度。
- 播放页视频信息会展示来源网盘类型，并提供删除入口。被删除或被隐藏且保留源文件的视频会进入黑名单，不会再出现在首页、列表、搜索和详情接口中。已删除源文件不再保留黑名单记录；本地上传和自动去重记录不提供普通恢复；重复记录会指向保留视频。

## 预览视频生成

scanner 扫到新视频会把 `(driveID, videoID)` 丢进 worker 队列。worker 会先用 `ffprobe` 探测时长，再用 `ffmpeg` 抽封面和生成无声预览视频：

```
ffmpeg -ss <起点> -headers "UA/Cookie/Referer" -i <直链> \
       -t 3 -an -vf scale=480:-2 -c:v libx264 -preset veryfast -crf 28 \
       -movflags +faststart -y <local>.mp4
```

当前策略是每段固定 3 秒；30 秒以下最多 3 段，30 秒及以上固定 4 段；长视频在 20% 到 80% 区间均匀取段。生成的预览视频和封面都只保存在本地 `data/previews/`，不会回写到网盘；旧数据中的 `preview_file_id` 会被忽略。

服务启动或网盘重新挂载时，如果预览视频开关已开启，后端会把历史 `pending` 任务重新入队，避免重启后长期停在“待生成”。OneDrive 扫盘和直链生成预览视频 / 封面时可能触发 Microsoft Graph 429、`TooManyRequests`、`activityLimitReached` 或 throttled 文本；Google Drive 可能返回 429、`usageLimits`、`userRateLimitExceeded`、`downloadQuotaExceeded` 等限制标识。后端会识别这类错误并让当前网盘进入冷却期，保留任务为 `pending`，避免连续请求触发更严重限流。扫盘阶段会按 `Retry-After` 或默认冷却时间等待后继续当前目录。

前端卡片的 `previewSrc` 统一指向 `/p/preview/<videoID>`，后端只从本地 `preview_local` 文件读取。

## 验证

```bash
# 前端，在仓库根目录执行
npm run lint
npm run build
node --test tests/previewIntent.test.ts

# 后端，在 backend/ 执行
go test ./... -count=1
```

## 部署到 Linux

推荐先使用根目录的预编译安装脚本：

```bash
sudo bash install.sh
```

它会从 GitHub Release 下载预编译包，安装运行依赖、写入 systemd 服务并启动。下面是手动部署方式，适合你想自己接管构建和服务管理时使用。

```bash
# 交叉编译
GOOS=linux GOARCH=amd64 go build -o video-server ./cmd/server

# 目标机
sudo apt install ffmpeg
scp video-server user@host:/opt/video-site/
ssh user@host
cd /opt/video-site
cp config.example.yaml config.yaml
# 改密码、监听地址
./video-server
```

配 systemd + nginx 反代到 `/` 和 `/api`、`/p`、`/admin`。nginx 需要按上面的示例传递真实 IP，否则登录失败封禁会封到 nginx 或本机地址。
