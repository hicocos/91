import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import test from "node:test";

const layout = readFileSync(new URL("../src/admin/AdminLayout.tsx", import.meta.url), "utf8");
const app = readFileSync(new URL("../src/App.tsx", import.meta.url), "utf8");
const css = readFileSync(new URL("../src/styles/admin.css", import.meta.url), "utf8");
const docsUrl = new URL("../src/admin/MountDocsPage.tsx", import.meta.url);

test("system navigation places mount docs immediately after theme on desktop and exposes it on mobile", () => {
  assert.match(
    layout,
    /to="\/admin\/theme"[\s\S]*?admin-nav__title">主题外观[\s\S]*?to="\/admin\/mount-docs"[\s\S]*?admin-nav__title">挂载文档[\s\S]*?检查更新[\s\S]*?退出登录/,
  );
  assert.match(layout, /admin-sidebar__mobile-panel[\s\S]*?to="\/admin\/mount-docs"[\s\S]*?挂载文档/);
});

test("admin router lazy loads the protected in-app mount docs page", () => {
  assert.match(app, /const MountDocsPage = lazy/);
  assert.match(app, /path="mount-docs"[\s\S]*?<MountDocsPage \/>/);
  assert.equal(existsSync(docsUrl), true);
});

test("mount docs are source-backed for every currently guided provider and operational restriction", () => {
  const docs = readFileSync(docsUrl, "utf8");
  for (const heading of ["PikPak", "OneDrive", "Google Drive", "WebDAV", "S3 兼容存储", "本地存储", "挂载后怎么使用"]) {
    assert.match(docs, new RegExp(heading));
  }
  assert.match(docs, /\/admin\/api\/storage\/oauth\/onedrive\/callback/);
  assert.match(docs, /\/admin\/api\/storage\/oauth\/googledrive\/callback/);
  assert.match(docs, /ALLOW_PRIVATE_STORAGE_ENDPOINTS/);
  assert.match(docs, /ALLOW_INSECURE_STORAGE_ENDPOINTS/);
  assert.match(docs, /S3[\s\S]*只读/);
  assert.match(docs, /PikPak[\s\S]*Refresh Token/);
  assert.match(docs, /Captcha Token/);
  assert.match(docs, /平台必须与 Token 来源一致/);
  assert.match(docs, /扫描目录前缀/);
  assert.match(docs, /测试并添加/);
});

test("mount docs use the existing admin visual language and include narrow viewport rules", () => {
  const docs = readFileSync(docsUrl, "utf8");
  assert.match(docs, /className="admin-mount-docs"/);
  assert.match(docs, /className="admin-mount-docs__toc"/);
  assert.match(css, /\.admin-mount-docs\s*\{/);
  assert.match(css, /@media \(max-width:\s*767px\)[\s\S]*\.admin-mount-docs__layout/s);
  assert.match(css, /overflow-wrap:\s*anywhere/);
}
);