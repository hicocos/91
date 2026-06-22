import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const adminCss = readFileSync(
  new URL("../src/styles/admin.css", import.meta.url),
  "utf8"
);
const videosPageSource = readFileSync(
  new URL("../src/admin/VideosPage.tsx", import.meta.url),
  "utf8"
);

function ruleBody(css: string, selector: string): string {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const match = css.match(new RegExp(`${escapedSelector}\\s*\\{([^}]*)\\}`));
  assert.ok(match, `Expected CSS rule for ${selector}`);
  return match[1];
}

function allRuleBodies(css: string, selector: string): string {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return Array.from(css.matchAll(new RegExp(`${escapedSelector}\\s*\\{([^}]*)\\}`, "g")))
    .map((match) => match[1])
    .join("\n");
}

// ruleBodyByContains 处理 CSS 里"多 selector 共享 body"的合并写法：
//   .a, .b, .c {
//     ...
//   }
// 上面的 `.b` 用直接的 `selector\s*\{` 正则匹不到。这里改成"找到任何包含目标
// selector 的连续 selector 列表（可含逗号 + 空白），紧跟一个 { ... } body"。
//
// 仅支持 body 内不再嵌套 `{}`（admin.css 没有 nesting，足够）。
function ruleBodyByContains(css: string, needle: string): string {
  const escapedNeedle = needle.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const re = new RegExp(`([^{}]*${escapedNeedle}[^{}]*)\\{([^}]*)\\}`, "g");
  const bodies: string[] = [];
  for (const m of css.matchAll(re)) {
    bodies.push(m[2]);
  }
  assert.ok(bodies.length > 0, `Expected at least one CSS rule containing ${needle}`);
  return bodies.join("\n");
}

function mobileCss(): string {
  const marker = "@media (max-width: 768px)";
  const start = adminCss.indexOf(marker);
  assert.notEqual(start, -1, "Expected mobile admin media query");
  return adminCss.slice(start);
}

test("admin login card fits narrow phone screens", () => {
  const body = ruleBody(adminCss, ".admin-login__card");

  // 桌面规则就用 min(...) 让窄屏自然适配；具体上限以 CSS 当前值为准（400px），
  // 关键是 `min(<某值>, 100%)` + `box-sizing: border-box`。
  assert.match(body, /width\s*:\s*min\(\d+px,\s*100%\)/);
  assert.match(body, /box-sizing\s*:\s*border-box/);
});

test("admin tables scroll inside the mobile viewport", () => {
  const css = mobileCss();
  // 视频/标签等"长内容"表的 mobile 形态：用 `.admin-table:not(.admin-drives-table)`
  // 把它们改成 display:block 卡片栈；网盘表 .admin-drives-table 走另一组 1280px 媒体
  // 查询。这里只断"非 drives 表的 mobile 卡片化"。
  const body = ruleBody(css, ".admin-table:not(.admin-drives-table)");

  assert.match(body, /display\s*:\s*block/);
});

test("admin video filter select uses an aligned custom arrow", () => {
  const select = ruleBody(adminCss, ".admin-videos-filter__select");
  const icon = ruleBody(adminCss, ".admin-videos-filter__select-icon");
  const mobileWrap = ruleBodyByContains(mobileCss(), ".admin-videos-filter__select-wrap");

  assert.match(select, /appearance\s*:\s*none/);
  assert.match(select, /padding\s*:\s*0\s+36px\s+0\s+var\(--space-3\)/);
  assert.match(icon, /top\s*:\s*50%/);
  assert.match(icon, /right\s*:\s*12px/);
  assert.match(icon, /transform\s*:\s*translateY\(-50%\)/);
  assert.match(mobileWrap, /flex\s*:\s*1\s+1\s+100%/);
});

test("admin video bulk actions use semantic theme colors", () => {
  const base = ruleBody(adminCss, ".admin-videos-bulk-actions__btn");
  const primary = ruleBody(adminCss, ".admin-videos-bulk-actions__btn.is-primary");
  const danger = ruleBody(adminCss, ".admin-videos-bulk-actions__btn.is-danger");
  const dangerHover = ruleBody(adminCss, ".admin-videos-bulk-actions__btn.is-danger:hover:not(:disabled)");
  const bulkBodies = [base, primary, danger, dangerHover].join("\n");

  assert.match(videosPageSource, /className="admin-btn is-primary admin-videos-bulk-actions__btn"/);
  assert.match(videosPageSource, /className="admin-btn is-danger admin-videos-bulk-actions__btn"/);
  assert.match(primary, /var\(--accent-glow\)/);
  assert.match(danger, /background\s*:\s*var\(--danger-soft\)/);
  assert.match(danger, /border-color\s*:\s*var\(--danger\)/);
  assert.match(danger, /color\s*:\s*var\(--danger\)/);
  assert.match(dangerHover, /background\s*:\s*var\(--danger\)/);
  assert.doesNotMatch(bulkBodies, /#ff5b8a|#fff6f9|rgba\(255,\s*91,\s*138/);
});

test("admin video list summary stays below filter controls", () => {
  const toolbar = ruleBody(adminCss, ".admin-videos-list-toolbar");

  assert.match(toolbar, /margin\s*:\s*var\(--space-2\)\s+0\s+var\(--space-4\)/);
  assert.doesNotMatch(toolbar, /margin\s*:\s*-/);
});

test("admin table action headers center-align with action buttons", () => {
  const actionHeader = ruleBody(adminCss, ".admin-table th.is-actions");
  const actionCell = ruleBody(adminCss, ".admin-table td.is-actions");

  assert.match(actionHeader, /text-align\s*:\s*center/);
  assert.match(actionCell, /text-align\s*:\s*center/);
});

test("blacklist restore action uses a light button style", () => {
  const restoreButton = ruleBody(adminCss, ".admin-blacklist-restore-btn");

  assert.match(videosPageSource, /className="admin-btn admin-blacklist-restore-btn"/);
  assert.match(restoreButton, /background\s*:\s*var\(--accent-softer\)/);
  assert.match(restoreButton, /color\s*:\s*var\(--accent\)/);
  assert.doesNotMatch(restoreButton, /background\s*:\s*var\(--accent\)/);
});

test("blacklist duplicate reason renders as a compact pill", () => {
  const pill = ruleBody(adminCss, ".admin-blacklist-reason-pill");

  assert.match(videosPageSource, /admin-blacklist-reason-pill/);
  assert.match(videosPageSource, /重复文件/);
  assert.match(pill, /border-radius\s*:\s*999px/);
  assert.match(pill, /white-space\s*:\s*nowrap/);
});

test("admin video management controls wrap instead of covering text on mobile", () => {
  const css = mobileCss();
  const paginationInfo = allRuleBodies(css, ".admin-table-pagination__info");
  const bulkActions = allRuleBodies(css, ".admin-videos-bulk-actions");
  const bulkCount = allRuleBodies(css, ".admin-videos-bulk-actions__count");
  const bulkButton = allRuleBodies(css, ".admin-videos-bulk-actions__btn");
  const blacklistName = ruleBody(
    css,
    '.admin-blacklist-table:not(.admin-drives-table) td[data-label="文件名"]'
  );
  const blacklistTime = ruleBody(
    css,
    '.admin-blacklist-table:not(.admin-drives-table) td[data-label="拉黑时间"]'
  );
  const blacklistActions = ruleBody(
    css,
    ".admin-blacklist-table:not(.admin-drives-table) td.is-actions"
  );
  const blacklistActionsLabel = ruleBody(
    css,
    ".admin-blacklist-table:not(.admin-drives-table) td.is-actions::before"
  );
  const blacklistActionButton = ruleBody(
    css,
    ".admin-blacklist-table:not(.admin-drives-table) td.is-actions .admin-btn"
  );

  assert.match(paginationInfo, /flex\s*:\s*1\s+0\s+100%/);
  assert.match(bulkActions, /flex-wrap\s*:\s*wrap/);
  assert.match(bulkCount, /flex\s*:\s*1\s+0\s+100%/);
  assert.match(bulkButton, /min-width\s*:\s*0/);
  assert.match(blacklistName, /grid-column\s*:\s*1\s*\/\s*-1/);
  assert.match(blacklistTime, /grid-column\s*:\s*1/);
  assert.match(blacklistActions, /grid-column\s*:\s*2/);
  assert.match(blacklistActions, /justify-content\s*:\s*flex-end/);
  assert.match(blacklistActionsLabel, /content\s*:\s*none/);
  assert.match(blacklistActionButton, /white-space\s*:\s*normal/);
});

test("admin loading spinner rotates around icon center", () => {
  const spinner = ruleBody(adminCss, ".admin-spin");
  const reducedMotion = ruleBodyByContains(adminCss, ".admin-sidebar__check-update:disabled svg");

  assert.match(spinner, /animation\s*:\s*admin-update-spin\s+0\.9s\s+linear\s+infinite/);
  assert.match(spinner, /transform-box\s*:\s*fill-box/);
  assert.match(spinner, /transform-origin\s*:\s*center/);
  assert.match(spinner, /will-change\s*:\s*transform/);
  assert.match(reducedMotion, /animation-duration\s*:\s*0\.9s\s*!important/);
});

test("mobile video management uses compact theme-aware video cards", () => {
  const css = mobileCss();
  const card = ruleBody(css, ".admin-videos-table:not(.admin-drives-table) tr");
  const title = ruleBody(css, ".admin-videos-table:not(.admin-drives-table) td[data-label=\"标题\"]");
  const checkbox = ruleBody(css, ".admin-videos-table:not(.admin-drives-table) td.is-checkbox");
  const label = ruleBody(css, ".admin-videos-table:not(.admin-drives-table) td::before");
  const titleCell = ruleBody(css, ".admin-videos-table:not(.admin-drives-table) .admin-video-title-cell");
  const thumb = ruleBody(css, ".admin-videos-table:not(.admin-drives-table) .admin-video-thumb-wrap");
  const titleText = ruleBody(css, ".admin-videos-table:not(.admin-drives-table) .admin-video-title");
  const pills = ruleBody(css, ".admin-videos-table:not(.admin-drives-table) .admin-video-filemeta-pills");
  const authorColumn = ruleBodyByContains(css, ".admin-videos-table:not(.admin-drives-table) td[data-label=\"作者\"]");
  const sourceColumn = ruleBodyByContains(css, ".admin-videos-table:not(.admin-drives-table) td[data-label=\"来源\"]");
  const durationColumn = ruleBodyByContains(css, ".admin-videos-table:not(.admin-drives-table) td[data-label=\"时长\"]");
  const previewColumn = ruleBodyByContains(css, ".admin-videos-table:not(.admin-drives-table) td[data-label=\"预览视频\"]");
  const actions = ruleBody(css, ".admin-videos-table:not(.admin-drives-table) td.is-actions");
  const actionsLabel = ruleBody(css, ".admin-videos-table:not(.admin-drives-table) td.is-actions::before");
  const checkboxLabel = ruleBodyByContains(css, ".admin-videos-table:not(.admin-drives-table) td.is-checkbox::before");
  const checkboxButton = ruleBody(css, ".admin-videos-table:not(.admin-drives-table) .admin-table-checkbox-btn");
  const status = ruleBody(css, ".admin-videos-table:not(.admin-drives-table) .admin-status");
  const statusDot = ruleBody(css, ".admin-videos-table:not(.admin-drives-table) .admin-status::before");
  const actionButton = ruleBody(css, ".admin-videos-table:not(.admin-drives-table) td.is-actions .admin-btn");
  const dangerButton = ruleBody(css, ".admin-videos-table:not(.admin-drives-table) td.is-actions .admin-btn.is-danger");

  assert.match(card, /--admin-video-card-bg\s*:\s*var\(--bg-surface\)/);
  assert.match(card, /background\s*:\s*var\(--admin-video-card-bg\)/);
  assert.match(card, /border-radius\s*:\s*14px/);
  assert.match(card, /padding\s*:\s*12px\s+14px/);
  assert.match(card, /grid-template-columns\s*:\s*repeat\(12,\s*minmax\(0,\s*1fr\)\)/);
  assert.match(card, /gap\s*:\s*0\s+10px/);
  assert.match(css, /:root:not\(\[data-theme="pink"\]\)\s+\.admin-videos-table:not\(\.admin-drives-table\)\s+tr\s*\{[^}]*--admin-video-card-bg\s*:\s*#1e1e1e/s);
  assert.match(css, /:root\[data-theme="pink"\]\s+\.admin-videos-table:not\(\.admin-drives-table\)\s+tr\s*\{/);
  assert.match(checkbox, /grid-column\s*:\s*1\s*\/\s*4/);
  assert.match(checkbox, /grid-row\s*:\s*3/);
  assert.match(checkbox, /display\s*:\s*flex/);
  assert.match(checkboxLabel, /content\s*:\s*none/);
  assert.match(checkboxButton, /width\s*:\s*100%/);
  assert.match(checkboxButton, /height\s*:\s*32px/);
  assert.match(title, /padding-left\s*:\s*0/);
  assert.match(title, /min-height\s*:\s*72px/);
  assert.match(label, /font-size\s*:\s*10px/);
  assert.match(label, /letter-spacing\s*:\s*0\.06em/);
  assert.match(titleCell, /grid-template-columns\s*:\s*clamp\(104px,\s*32vw,\s*156px\)\s+minmax\(0,\s*1fr\)/);
  assert.match(thumb, /aspect-ratio\s*:\s*16\s*\/\s*9/);
  assert.match(thumb, /border-radius\s*:\s*8px/);
  assert.match(titleText, /-webkit-line-clamp\s*:\s*2/);
  assert.match(titleText, /overflow-wrap\s*:\s*anywhere/);
  assert.match(videosPageSource, /loading="lazy"\s+decoding="async"/);
  assert.match(videosPageSource, /className="admin-video-title" title=\{v\.title\}/);
  assert.match(pills, /display\s*:\s*flex/);
  assert.doesNotMatch(videosPageSource, /admin-video-filemeta-pill is-category/);
  assert.doesNotMatch(css, /admin-video-card-category/);
  assert.match(authorColumn, /display\s*:\s*none/);
  assert.match(sourceColumn, /grid-row\s*:\s*2/);
  assert.match(sourceColumn, /grid-column\s*:\s*1\s*\/\s*5/);
  assert.match(sourceColumn, /justify-items\s*:\s*start/);
  assert.match(sourceColumn, /text-overflow\s*:\s*ellipsis/);
  assert.match(durationColumn, /grid-row\s*:\s*2/);
  assert.match(durationColumn, /grid-column\s*:\s*5\s*\/\s*9/);
  assert.match(durationColumn, /justify-items\s*:\s*center/);
  assert.match(previewColumn, /grid-row\s*:\s*2/);
  assert.match(previewColumn, /grid-column\s*:\s*9\s*\/\s*-1/);
  assert.match(previewColumn, /justify-items\s*:\s*end/);
  assert.match(actions, /grid-column\s*:\s*4\s*\/\s*-1/);
  assert.match(actions, /grid-row\s*:\s*3/);
  assert.match(actions, /display\s*:\s*grid/);
  assert.match(actions, /grid-template-columns\s*:\s*repeat\(3,\s*minmax\(0,\s*1fr\)\)/);
  assert.match(actions, /gap\s*:\s*10px/);
  assert.match(actionsLabel, /content\s*:\s*none/);
  assert.match(status, /gap\s*:\s*0/);
  assert.match(statusDot, /content\s*:\s*none/);
  assert.doesNotMatch(sourceColumn, /border-left/);
  assert.match(actionButton, /width\s*:\s*100%/);
  assert.match(actionButton, /height\s*:\s*32px/);
  assert.match(actionButton, /justify-content\s*:\s*center/);
  assert.match(actionButton, /border-radius\s*:\s*8px/);
  assert.match(dangerButton, /border-color\s*:\s*var\(--admin-video-card-danger-border\)/);
  assert.match(dangerButton, /color\s*:\s*var\(--admin-video-card-danger\)/);
});

test("video edit modal stays focused on common metadata", () => {
  assert.match(videosPageSource, /ariaLabel="编辑视频"/);
  assert.doesNotMatch(videosPageSource, /title=\{`编辑视频 ·/);
  assert.doesNotMatch(videosPageSource, /const \[badges, setBadges\]/);
  assert.doesNotMatch(videosPageSource, /const \[thumbnail, setThumbnail\]/);
  assert.doesNotMatch(videosPageSource, /const \[quality, setQuality\]/);
  assert.doesNotMatch(videosPageSource, /video-badges/);
  assert.doesNotMatch(videosPageSource, /video-quality/);
  assert.doesNotMatch(videosPageSource, /video-thumbnail/);
  assert.doesNotMatch(videosPageSource, /徽标（/);
  assert.doesNotMatch(videosPageSource, /封面 URL/);
  assert.doesNotMatch(videosPageSource, /封面预览/);
  assert.doesNotMatch(videosPageSource, /badges:\s*splitList\(badges\)/);
  assert.doesNotMatch(videosPageSource, /thumbnail:\s*thumbnail\.trim\(\)/);
  assert.doesNotMatch(videosPageSource, /quality:\s*quality\.trim\(\)/);
});

test("admin modals and action footers adapt on mobile", () => {
  const css = mobileCss();

  // .admin-modal 桌面段已用 `width: min(620px, 100%)`，窄屏自然 100%；mobile 段
  // 只重写 max-height，所以这里断桌面规则即可。
  assert.match(ruleBody(adminCss, ".admin-modal"), /width\s*:\s*min\(\d+px,\s*100%\)/);
  assert.match(ruleBody(adminCss, ".admin-modal.admin-modal--crawler"), /width\s*:\s*min\(1080px,\s*100%\)/);
  // 多按钮 footer 在 mobile 下要换行避免溢出。
  assert.match(allRuleBodies(css, ".admin-modal__footer"), /flex-wrap\s*:\s*wrap/);
  // 删除/放弃类确认弹窗在 mobile 下不能跟随通用 modal stretch 到顶部。
  const confirmModal = ruleBody(css, ".admin-modal--delete-confirm");
  assert.match(confirmModal, /align-self\s*:\s*center/);
  assert.match(confirmModal, /justify-self\s*:\s*center/);
  assert.match(ruleBody(adminCss, ".admin-modal__header.is-titleless"), /justify-content\s*:\s*flex-end/);
  // 表单 input/select/textarea 在 mobile 下铺满。规则用逗号合并写法（多 selector
  // 共享 body），所以走 ruleBodyByContains 而不是简单正则。
  assert.match(ruleBodyByContains(css, ".admin-form__row input"), /width\s*:\s*100%/);
});

test("mobile admin top navigation stays compact", () => {
  const css = mobileCss();

  assert.match(ruleBody(css, ".admin-shell"), /display\s*:\s*flex/);
  assert.match(ruleBody(css, ".admin-shell"), /flex-direction\s*:\s*column/);
  assert.match(ruleBody(css, ".admin-sidebar"), /height\s*:\s*48px/);
  assert.match(ruleBody(css, ".admin-sidebar"), /min-height\s*:\s*48px/);
  assert.match(ruleBody(css, ".admin-nav"), /align-items\s*:\s*center/);
  assert.match(ruleBody(css, ".admin-nav__link"), /height\s*:\s*34px/);
  assert.match(ruleBody(css, ".admin-nav__link"), /line-height\s*:\s*1/);
  assert.match(ruleBody(css, ".admin-nav__link"), /flex\s*:\s*0\s+0\s+auto/);
  assert.match(ruleBody(css, ".admin-main"), /padding\s*:\s*var\(--space-2\)\s+var\(--space-3\)\s+var\(--space-4\)/);
  assert.match(ruleBody(css, ".admin-page__header"), /margin-bottom\s*:\s*var\(--space-3\)/);
});
