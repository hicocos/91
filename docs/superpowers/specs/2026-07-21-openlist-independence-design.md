# 91 去 OpenList 依赖设计

## 目标

让 `/www/91` 的挂载、扫描、播放、上传和凭据续期不依赖 OpenList 服务、OpenList 私有 API或 OpenListTeam Go 模块，同时保留标准 WebDAV 互操作能力。

## 边界

- OneDrive 只直接请求 Microsoft OAuth 与 Graph API；删除 `api.oplist.org` 代刷新及其兼容入口。
- WoPan 使用 91 仓库内的内部客户端包；来源为当前已 vendor 的 Apache-2.0 `wopan-sdk-go v0.2.0`，保留许可证和来源声明。
- 不复制 OpenList 主仓 AGPL-3.0 实现；`/www/tem/openlist` 仅用于行为审计与协议参考。
- WebDAV 保持厂商中立，管理员仍可自行连接任意标准 WebDAV 服务端。
- 不修改现有数据库记录；当前线上只有 PikPak 记录，不涉及 OneDrive/WoPan 凭据迁移。

## 数据流

管理后台配置账户后，91 继续使用现有 provider registry、probe-before-save、catalog、attach 生命周期和 `drives.Drive` 数据面。OneDrive token 由 Microsoft endpoint 刷新并通过 `MergeDriveCredentials` 回写。WoPan driver 仅把 import 从外部模块替换为仓库内部客户端，接口和远端协议保持不变。

## 验证

- OneDrive：Microsoft POST 刷新、缺少 client ID 失败、过期 token 刷新后单次重放。
- WoPan：原有 driver 测试及内部客户端编译通过。
- 独立性合约：生产源码、Go 模块和 vendor manifest 无 OpenList/OpenListTeam 运行或编译依赖；通用 WebDAV 文档不绑定厂商。
- 后端全量 test/vet，前端 lint/test/build，Docker 镜像重建、容器重建、健康检查、首页和日志检查。
