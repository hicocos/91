import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const apiSource = readFileSync(
  new URL("../src/admin/api.ts", import.meta.url),
  "utf8"
);
const tagsPageSource = readFileSync(
  new URL("../src/admin/TagsPage.tsx", import.meta.url),
  "utf8"
);
const videosPageSource = readFileSync(
  new URL("../src/admin/VideosPage.tsx", import.meta.url),
  "utf8"
);

test("admin tags expose editable match rules and background jobs", () => {
  assert.match(apiSource, /export type TagMatchRules/);
  assert.match(apiSource, /export function updateTag/);
  assert.match(apiSource, /export function startTagRetag/);
  assert.match(apiSource, /export function getTagJobStatus/);
  assert.match(apiSource, /autoGenerateTagsEnabled: boolean/);
  assert.doesNotMatch(apiSource, /startTagLlmRun/);
  assert.doesNotMatch(apiSource, /llmEnabled|llmPending/);
  assert.match(tagsPageSource, /编辑标签：/);
  assert.match(tagsPageSource, /包含词（子串）/);
  assert.match(tagsPageSource, /识别文件名和标题中的番号/);
  assert.match(tagsPageSource, /重新整理所有标签/);
  assert.match(tagsPageSource, /重新整理所有标签[\s\S]*\{selectMode \? "退出批量" : "批量删除"\}/);
  assert.doesNotMatch(tagsPageSource, /<RefreshCw size=\{13\} \/> 刷新/);
  assert.match(tagsPageSource, /自动生成标签/);
  assert.match(tagsPageSource, /autoGenerateTagsEnabled/);
  assert.match(tagsPageSource, /admin-tag-setting-toggle__switch/);
  assert.match(tagsPageSource, /role="switch"/);
  assert.match(tagsPageSource, /onClick=\{toggleAutoGenerateTags\}/);
  assert.doesNotMatch(tagsPageSource, /admin-tag-setting-toggle__hint/);
  assert.doesNotMatch(tagsPageSource, /admin-tag-setting-toggle__body/);
  assert.doesNotMatch(tagsPageSource, /关闭后扫描只匹配已有标签。/);
  assert.doesNotMatch(tagsPageSource, /<strong>\{autoGenerateTagsEnabled \? "开启" : "关闭"\}<\/strong>/);
  assert.doesNotMatch(tagsPageSource, /AI 辅助打标|AI 打标|tagging\.llm/);
  assert.match(tagsPageSource, /const TAG_SOURCE_FILTERS = \["builtin", "user", "generated"\]/);
  assert.match(tagsPageSource, /return "爬虫脚本"/);
  assert.match(tagsPageSource, /sourceLabel\(tag\.source, tag\)/);
  assert.match(tagsPageSource, /return "自动生成"/);
  assert.doesNotMatch(tagsPageSource, /return "系统"/);
  assert.doesNotMatch(tagsPageSource, /return "旧数据"/);
  assert.doesNotMatch(tagsPageSource, /tag\.source !== "system"/);
  assert.doesNotMatch(tagsPageSource, /tag\.source === "system"\) return/);
});

test("admin tags batch delete runs deletions sequentially", () => {
  assert.match(tagsPageSource, /for \(const id of ids\) \{/);
  assert.match(tagsPageSource, /await api\.deleteTag\(id\);/);
  assert.doesNotMatch(
    tagsPageSource,
    /Promise\.allSettled\(\s*ids\.map\(\(id\) => api\.deleteTag\(id\)\)\s*\)/
  );
});

test("admin videos render tag assignment source and evidence", () => {
  assert.match(apiSource, /tagSources\?: Record<string, string>/);
  assert.match(apiSource, /tagEvidence\?: Record<string, string>/);
  assert.match(videosPageSource, /data-source=\{v\.tagSources\?\.\[t\]/);
  assert.match(videosPageSource, /tagAssignmentSourceLabel/);
  assert.match(videosPageSource, /tagAssignmentTitle/);
  assert.match(videosPageSource, /video\.tagEvidence\?\.\[tag\.label\]/);
});
