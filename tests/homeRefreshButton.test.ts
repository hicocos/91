import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const homePageSource = readFileSync(
  new URL("../src/pages/HomePage.tsx", import.meta.url),
  "utf8"
);
const tagCloudSource = readFileSync(
  new URL("../src/components/TagCloud.tsx", import.meta.url),
  "utf8"
);
const layoutCss = readFileSync(
  new URL("../src/styles/layout.css", import.meta.url),
  "utf8"
);
const appShellSource = readFileSync(
  new URL("../src/components/AppShell.tsx", import.meta.url),
  "utf8"
);
const backToTopSource = readFileSync(
  new URL("../src/components/BackToTop.tsx", import.meta.url),
  "utf8"
);

function ruleBody(css: string, selector: string): string {
  const escapedSelector = selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  const match = css.match(new RegExp(`${escapedSelector}\\s*\\{([^}]*)\\}`));
  assert.ok(match, `Expected CSS rule for ${selector}`);
  return match[1];
}

test("home page refresh button shares back-to-top slot until back-to-top is visible", () => {
  assert.match(homePageSource, /import \{ Film, RefreshCw \} from "lucide-react"/);
  assert.match(homePageSource, /const refreshHome = useCallback\(async \(\) =>/);
  assert.match(homePageSource, /fetchHomeVideos\(excludeIds\)/);
  assert.match(homePageSource, /fetchListing\(1,\s*DESKTOP_COUNT,\s*\{ sort: "latest", includeTotal: false \}\)/);
  assert.match(homePageSource, /className=\{`home-refresh \$\{refreshing \? "is-refreshing" : ""\}`\}/);
  assert.match(homePageSource, /aria-label="刷新首页"/);
  assert.match(homePageSource, /<RefreshCw size=\{18\} \/>/);

  const refresh = ruleBody(layoutCss, ".home-refresh");
  const shiftedRefresh = ruleBody(layoutCss, ".app-shell.is-back-to-top-visible .home-refresh");
  const backToTop = ruleBody(layoutCss, ".back-to-top");
  assert.match(refresh, /position\s*:\s*fixed/);
  assert.match(refresh, /bottom\s*:\s*24px/);
  assert.match(backToTop, /bottom\s*:\s*24px/);
  assert.match(shiftedRefresh, /bottom\s*:\s*80px/);
  assert.match(refresh, /z-index\s*:\s*var\(--z-overlay\)/);
  assert.doesNotMatch(layoutCss, /\.home-refresh\.is-visible/);

  assert.match(appShellSource, /const \[backToTopVisible,\s*setBackToTopVisible\] = useState\(false\)/);
  assert.match(appShellSource, /backToTopVisible \? "is-back-to-top-visible" : ""/);
  assert.match(appShellSource, /<BackToTop onVisibilityChange=\{setBackToTopVisible\} \/>/);
  assert.match(backToTopSource, /onVisibilityChange\?: \(visible: boolean\) => void/);
  assert.match(backToTopSource, /onVisibilityChange\?\.\(nextVisible\)/);
});

test("home page hides empty tag cloud and uses one empty library state", () => {
  assert.match(tagCloudSource, /const visibleTags = useMemo/);
  assert.match(tagCloudSource, /typeof tag\.count !== "number" \|\| tag\.count > 0/);
  assert.match(tagCloudSource, /if \(visibleTags\.length === 0\) return null/);
  assert.match(tagCloudSource, /const row1 = visibleTags\.filter/);

  assert.match(homePageSource, /const homeLoading = rankingLoading \|\| latestLoading/);
  assert.match(homePageSource, /const hasAnyVideos = ranking\.length > 0 \|\| latest\.length > 0/);
  assert.match(homePageSource, /const showEmptyHome = !homeLoading && !hasAnyVideos/);
  assert.match(homePageSource, /className="home-empty"/);
  assert.match(homePageSource, /当前还没有可播放的视频/);

  const empty = ruleBody(layoutCss, ".home-empty");
  assert.match(empty, /min-height\s*:\s*240px/);
  assert.match(empty, /border\s*:\s*1px dashed var\(--border-default\)/);
  assert.match(empty, /border-radius\s*:\s*8px/);
});
