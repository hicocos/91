import { useEffect } from "react";
import { AlertTriangle, CheckCircle2, ExternalLink, FolderOpen, HardDrive, KeyRound, Network } from "lucide-react";

const sections = [
  ["start", "开始挂载"],
  ["pikpak", "PikPak"],
  ["onedrive", "OneDrive"],
  ["googledrive", "Google Drive"],
  ["webdav", "WebDAV"],
  ["s3", "S3 兼容存储"],
  ["local", "本地存储"],
  ["after", "挂载后怎么使用"],
] as const;

const CALLBACK_PATHS = {
  onedrive: "/admin/api/storage/oauth/onedrive/callback",
  googledrive: "/admin/api/storage/oauth/googledrive/callback",
} as const;

function CallbackURL({ provider }: { provider: keyof typeof CALLBACK_PATHS }) {
  const value = `${window.location.origin}${CALLBACK_PATHS[provider]}`;
  return <code className="admin-mount-docs__callback">{value}</code>;
}

export function MountDocsPage() {
  useEffect(() => {
    document.title = "挂载文档 - 后台管理";
  }, []);

  return (
    <article className="admin-mount-docs">
      <header className="admin-mount-docs__hero">
        <div>
          <span className="admin-mount-docs__eyebrow">存储接入指南</span>
          <h1>挂载网盘与存储</h1>
          <p>准备凭据、完成连接测试，然后让系统从指定目录扫描视频。每个挂载独立保存，可同时管理多个网盘。</p>
        </div>
        <HardDrive size={42} aria-hidden="true" />
      </header>

      <div className="admin-mount-docs__layout">
        <nav className="admin-mount-docs__toc" aria-label="挂载文档目录">
          <strong>本文目录</strong>
          {sections.map(([id, label]) => <a key={id} href={`#${id}`}>{label}</a>)}
        </nav>

        <div className="admin-mount-docs__content">
          <section id="start" className="admin-mount-docs__section">
            <h2><FolderOpen size={20} />开始挂载</h2>
            <ol>
              <li>进入“网盘管理”，点击“添加网盘”。</li>
              <li>选择存储类型，填写名称和页面默认展示的必要参数。</li>
              <li>OneDrive 与 Google Drive 先点击“授权连接”；其他类型直接填写凭据。</li>
              <li>点击“测试并添加”。只有连接和扫描目录验证通过后才会保存。</li>
            </ol>
            <div className="admin-mount-docs__notice is-info">
              <CheckCircle2 size={18} />
              <p>“高级设置”用于手动 Token、SharePoint、Shared Drive、临时 S3 凭据或自定义根目录。普通个人网盘通常无需展开。</p>
            </div>
          </section>

          <section id="pikpak" className="admin-mount-docs__section">
            <h2>PikPak</h2>
            <p>推荐填写 Refresh Token 挂载，避免服务器账号密码登录触发验证码或“操作频繁”风控。平台必须与 Token 来源一致。</p>
            <ul>
              <li><strong>Web：</strong>使用 PikPak 官方网页端登录后，在浏览器存储中找到以 <code>credentials</code> 开头的记录并复制 Refresh Token。</li>
              <li><strong>Android：</strong>只使用从官方 Android 客户端取得的 Refresh Token；不要把 Web Token 配成 Android 平台。</li>
              <li><strong>账号密码：</strong>仍可用于首次登录并自动保存 Refresh Token，但服务器 IP 可能触发交互验证码。</li>
              <li><strong>Captcha Token：</strong>如果错误给出验证码页面，打开页面完成验证，从验证请求中取得 <code>captcha_token</code> 后回填，再测试保存。</li>
              <li><strong>Device ID：</strong>与 Token 一并取得时保持配套；没有时系统会稳定生成并在登录成功后保存。</li>
            </ul>
            <div className="admin-mount-docs__notice is-warning">
              <AlertTriangle size={18} />
              <p>若提示“操作频繁”，这是 PikPak 针对账号或服务器出口 IP 的临时限制；不要连续重复测试。可先在官方客户端完成登录，再改用匹配平台的 Refresh Token。</p>
            </div>
          </section>

          <section id="onedrive" className="admin-mount-docs__section">
            <h2>OneDrive</h2>
            <p>推荐使用 OAuth 授权。个人账号只需 Client ID；机密客户端或组织策略要求时再填写 Client Secret。</p>
            <h3>创建 Microsoft Entra 应用</h3>
            <ol>
              <li>打开 <a href="https://entra.microsoft.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade" target="_blank" rel="noreferrer">Microsoft Entra 应用注册 <ExternalLink size={13} /></a>，新建应用。</li>
              <li>支持的账户类型按实际账号选择；不确定时使用可支持个人 Microsoft 账号的类型。</li>
              <li>在“身份验证”中添加 Web 重定向 URI：</li>
            </ol>
            <CallbackURL provider="onedrive" />
            <ol start={4}>
              <li>复制“应用程序（客户端）ID”到添加表单，点击“授权连接”并登录最终需要挂载的账号。</li>
              <li>如使用 Client Secret，请复制“证书和密码”新建密码后的 <strong>值 Value</strong>，不要复制 Secret ID。</li>
            </ol>
            <h3>SharePoint</h3>
            <p>切换为 SharePoint 模式后，填写 Site ID 和 Drive ID。扫描起点仍可在高级设置中填写目录 Item ID；留空使用根目录。</p>
          </section>

          <section id="googledrive" className="admin-mount-docs__section">
            <h2>Google Drive</h2>
            <ol>
              <li>在 <a href="https://console.cloud.google.com/apis/library/drive.googleapis.com" target="_blank" rel="noreferrer">Google Cloud Console <ExternalLink size={13} /></a> 创建项目并启用 Google Drive API。</li>
              <li>配置 OAuth 权限请求页面。应用仍处于测试状态时，把授权账号加入“测试用户”。</li>
              <li>创建“Web 应用”OAuth 客户端，并添加下面的已获授权重定向 URI：</li>
            </ol>
            <CallbackURL provider="googledrive" />
            <ol start={4}>
              <li>填写“客户端 ID”和“客户端密钥”，点击“授权连接”，使用最终需要扫描的 Google 账号完成授权。</li>
              <li>展开“高级设置”，按照下面的场景填写挂载范围。</li>
              <li>点击“测试并添加”，保存后在网盘卡片中执行扫盘。</li>
            </ol>

            <h3>两个 ID 字段分别填写什么</h3>
            <div className="admin-mount-docs__table-wrap">
              <table>
                <thead><tr><th>字段</th><th>用途</th><th>什么时候填写</th></tr></thead>
                <tbody>
                  <tr><td><strong>共享云端硬盘（团队盘）ID</strong></td><td>告诉 Google API 要访问哪一个团队盘</td><td>仅团队盘填写；个人盘留空</td></tr>
                  <tr><td><strong>扫描起点文件夹 ID（可选）</strong></td><td>把扫描范围缩小到某个子文件夹</td><td>只扫描指定文件夹时填写；扫描整个个人盘或整个团队盘时留空</td></tr>
                </tbody>
              </table>
            </div>

            <h3>三种常见填写方式</h3>
            <ol>
              <li><strong>扫描整个“我的云端硬盘”：</strong>团队盘 ID 留空，扫描起点文件夹 ID 留空。</li>
              <li><strong>扫描整个团队盘：</strong>填写共享云端硬盘（团队盘）ID，扫描起点文件夹 ID 留空。系统会自动使用团队盘 ID 作为根目录，无需在两个字段重复填写。</li>
              <li><strong>只扫描团队盘中的某个文件夹：</strong>填写团队盘 ID，并在扫描起点文件夹 ID 中填写目标子文件夹的 ID。</li>
            </ol>

            <h3>从链接取得 ID</h3>
            <p>打开团队盘或目标文件夹，复制浏览器地址中 <code>folders/</code> 后面的部分。例如：</p>
            <pre><code>https://drive.google.com/drive/folders/0Axxxxxxxxxxxxxxxxx</code></pre>
            <p>应填写：<code>0Axxxxxxxxxxxxxxxxx</code>。不要填写完整网址，也不要包含 <code>folders/</code>。</p>
            <div className="admin-mount-docs__notice is-warning">
              <AlertTriangle size={18} />
              <p>授权账号必须是该团队盘的成员，并至少拥有读取目标文件和目录的权限。团队盘 ID 与子文件夹 ID 是不同概念，只有目标文件夹恰好是团队盘根目录时两者才会相同。</p>
            </div>

            <h3>扫描结果为 0 时</h3>
            <ul>
              <li>确认团队盘 ID 来自正确的团队盘链接，而不是个人盘文件夹。</li>
              <li>确认授权时登录的账号是该团队盘成员，并能在 Google Drive 网页中打开目标目录。</li>
              <li>扫描整个团队盘时，扫描起点文件夹 ID 应留空；只扫子目录时才填写子文件夹 ID。</li>
              <li>确认媒体文件位于当前扫描起点的 5 层目录以内，并使用系统支持的视频或音频扩展名。</li>
              <li>修改配置后重新测试连接，并再次执行扫盘。</li>
            </ul>
          </section>

          <section id="webdav" className="admin-mount-docs__section">
            <h2>WebDAV</h2>
            <p>填写完整 WebDAV 地址，例如 <code>https://dav.jianguoyun.com/dav/</code>。需要登录时选择“账号密码”，公开 DAV 才选择“匿名访问”。</p>
            <ul>
              <li><strong>用户名：</strong>服务商要求的账号或邮箱。</li>
              <li><strong>密码：</strong>坚果云等服务通常要求应用专用口令，而不是网页登录密码。</li>
              <li><strong>WebDAV 根目录：</strong>高级设置中的远端路径，例如 <code>/videos</code>；留空使用 <code>/</code>。</li>
            </ul>
          </section>

          <section id="s3" className="admin-mount-docs__section">
            <h2>S3 兼容存储</h2>
            <p>支持 AWS S3、Cloudflare R2、MinIO 和兼容 S3 协议的对象存储。先选择服务类型，表单会填入常见 Region 与 Path Style 默认值。</p>
            <ul>
              <li><strong>Bucket：</strong>存储桶名称，不是公开访问域名。</li>
              <li><strong>Access Key / Secret Key：</strong>建议创建只允许目标 Bucket 的最小权限凭据。</li>
              <li><strong>Endpoint：</strong>AWS 可留空；R2 填 <code>https://账号ID.r2.cloudflarestorage.com</code>；MinIO 填服务 API 地址。</li>
              <li><strong>扫描目录前缀：</strong>必须填写，例如 <code>videos/</code>，系统不会扫描整个 Bucket 根目录。</li>
              <li><strong>Session Token：</strong>只有 STS 临时凭据需要，在高级设置填写。</li>
            </ul>
            <div className="admin-mount-docs__notice is-warning">
              <AlertTriangle size={18} />
              <p>S3 在本项目中是只读媒体来源：支持扫描、读取、播放及管理员显式删除源文件，不支持爬虫上传、创建目录或转码产物写回。</p>
            </div>
          </section>

          <section id="local" className="admin-mount-docs__section">
            <h2>本地存储</h2>
            <p>填写容器内可访问的绝对目录。Docker 部署时，宿主机目录必须先通过 <code>docker-compose.yml</code> 的 <code>volumes</code> 映射进容器；不能直接填写只存在于宿主机、容器不可见的路径。</p>
          </section>

          <section className="admin-mount-docs__section">
            <h2><Network size={20} />私网与 HTTP 地址</h2>
            <p>为防止 SSRF，WebDAV 与 S3 默认只允许公网 HTTPS Endpoint。如果要连接局域网 NAS、MinIO 或 HTTP 测试服务，在项目目录的 <code>docker-compose.yml</code> 中为服务增加：</p>
            <pre><code>{`environment:\n  ALLOW_PRIVATE_STORAGE_ENDPOINTS: "true"\n  ALLOW_INSECURE_STORAGE_ENDPOINTS: "true" # 仅使用 HTTP 时需要`}</code></pre>
            <p>修改后运行 <code>docker compose up -d --build</code>。只需要私网 HTTPS 时不要开启 <code>ALLOW_INSECURE_STORAGE_ENDPOINTS</code>。</p>
          </section>

          <section id="after" className="admin-mount-docs__section">
            <h2><KeyRound size={20} />挂载后怎么使用</h2>
            <ol>
              <li>添加成功后，在网盘卡片中打开详情并点击“开始扫盘”。</li>
              <li>扫描完成的视频进入主站；播放、封面和预览都绑定原网盘，不需要切换“活动账户”。</li>
              <li>修改凭据时，密码或 Token 留空表示保持原值；切换 WebDAV 匿名模式会明确清除旧登录凭据。</li>
              <li>需要缩小扫描范围时编辑挂载，在高级设置修改根目录或目录 ID，再重新扫盘。</li>
            </ol>
          </section>
        </div>
      </div>
    </article>
  );
}
