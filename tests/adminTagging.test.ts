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
const adminCss = readFileSync(
  new URL("../src/styles/admin.css", import.meta.url),
  "utf8"
);
const videosPageSource = readFileSync(
  new URL("../src/admin/VideosPage.tsx", import.meta.url),
  "utf8"
);

test("admin tags keep builtin, user, and auto-generated tag management", () => {
  assert.doesNotMatch(apiSource, /export type TagMatchRules/);
  assert.match(apiSource, /matchRules\?: \{/);
  assert.match(apiSource, /matchAvCode\?: boolean/);
  assert.match(apiSource, /avCodePrefixes\?: string\[\]/);
  assert.match(apiSource, /export function updateTag/);
  assert.match(apiSource, /updateTag\(id: number, aliases: string\[\]\)/);
  assert.doesNotMatch(apiSource, /export function startTagRetag/);
  assert.doesNotMatch(apiSource, /export function getTagJobStatus/);
  assert.doesNotMatch(apiSource, /autoGenerateTagsEnabled: boolean/);
  assert.doesNotMatch(apiSource, /startTagLlmRun/);
  assert.doesNotMatch(apiSource, /llmEnabled|llmPending/);
  assert.doesNotMatch(tagsPageSource, /编辑标签：/);
  assert.doesNotMatch(tagsPageSource, /<h1 className="admin-page__title">标签管理<\/h1>/);
  assert.match(tagsPageSource, /<div className="admin-tags-board">/);
  assert.match(tagsPageSource, /<aside className="admin-tags-filter-panel" aria-label="标签分类">/);
  assert.match(tagsPageSource, /<div className="admin-tags-main">/);
  assert.match(tagsPageSource, /admin-tags-filter-tab__text/);
  assert.doesNotMatch(tagsPageSource, /admin-tags-filter-tab__count/);
  assert.doesNotMatch(tagsPageSource, /aria-label=\{`\$\{label\} \(\$\{count\}\)`\}/);
  assert.doesNotMatch(tagsPageSource, /aria-label=\{`全部 \(\$\{stats\.total\}\)`\}/);
  assert.match(tagsPageSource, /添加标签/);
  assert.match(tagsPageSource, /onClick=\{openCreateModal\}/);
  assert.match(tagsPageSource, /className="admin-btn"\s+onClick=\{openCreateModal\}/);
  assert.doesNotMatch(tagsPageSource, /<Plus size=\{13\} \/> 新增标签/);
  assert.doesNotMatch(tagsPageSource, /className="admin-btn is-primary"\s+onClick=\{openCreateModal\}/);
  assert.match(tagsPageSource, /form="admin-create-tag-form"/);
  assert.doesNotMatch(tagsPageSource, /admin-card__title[\s\S]*新增标签/);
  assert.doesNotMatch(tagsPageSource, /系统不会再从文件名或标题自动创建标签/);
  assert.doesNotMatch(tagsPageSource, /包含词（子串）/);
  assert.doesNotMatch(tagsPageSource, /识别文件名和标题中的番号/);
  assert.doesNotMatch(tagsPageSource, /重新整理所有标签/);
  assert.doesNotMatch(tagsPageSource, /<RefreshCw size=\{13\} \/> 刷新/);
  assert.doesNotMatch(tagsPageSource, /自动生成标签/);
  assert.doesNotMatch(tagsPageSource, /autoGenerateTagsEnabled/);
  assert.doesNotMatch(tagsPageSource, /admin-tag-setting-toggle__switch/);
  assert.doesNotMatch(tagsPageSource, /role="switch"/);
  assert.doesNotMatch(tagsPageSource, /onClick=\{toggleAutoGenerateTags\}/);
  assert.doesNotMatch(tagsPageSource, /admin-tag-setting-toggle__hint/);
  assert.doesNotMatch(tagsPageSource, /admin-tag-setting-toggle__body/);
  assert.doesNotMatch(tagsPageSource, /关闭后扫描只匹配已有标签。/);
  assert.doesNotMatch(tagsPageSource, /<strong>\{autoGenerateTagsEnabled \? "开启" : "关闭"\}<\/strong>/);
  assert.doesNotMatch(tagsPageSource, /AI 辅助打标|AI 打标|tagging\.llm/);
  assert.match(tagsPageSource, /const TAG_SOURCE_FILTERS = \["builtin", "user", "generated"\]/);
  assert.match(tagsPageSource, /function tagSourceKey/);
  assert.match(tagsPageSource, /tag\.crawlerOwned \|\| tag\.source === "generated" \? "generated" : tag\.source/);
  assert.match(tagsPageSource, /source === "crawler" \|\| source === "generated"/);
  assert.match(tagsPageSource, /tagCardSourceLabel\(tag\)/);
  assert.doesNotMatch(tagsPageSource, /admin-tag-card__id/);
  assert.doesNotMatch(tagsPageSource, /#\{tag\.id\}/);
  assert.match(tagsPageSource, /function tagCardSourceLabel/);
  assert.match(tagsPageSource, /tag\.crawlerOwned \|\| tag\.source === "crawler"/);
  assert.match(tagsPageSource, /return "爬虫脚本"/);
  assert.match(tagsPageSource, /tag\.source === "generated"/);
  assert.match(tagsPageSource, /return "AV"/);
  assert.doesNotMatch(tagsPageSource, /const displayAliases = tagDisplayAliases\(tag\);/);
  assert.doesNotMatch(tagsPageSource, /admin-tag-card__aliases/);
  assert.doesNotMatch(tagsPageSource, /admin-tag-card__alias-pill/);
  assert.doesNotMatch(tagsPageSource, /function tagDisplayAliases/);
  assert.match(tagsPageSource, /tag\.matchRules\?\.avCodePrefixes/);
  assert.doesNotMatch(tagsPageSource, /function uniqueDisplayAliases/);
  assert.doesNotMatch(tagsPageSource, /系统内置车牌已自动参与匹配/);
  assert.match(tagsPageSource, /const TAG_DISPLAY_GROUP_ORDER: Record<string, number>/);
  assert.match(tagsPageSource, /function tagDisplayGroupKey/);
  assert.match(tagsPageSource, /function tagDisplayGroupRank/);
  assert.match(tagsPageSource, /if \(filterSource !== "all"\) return matches;/);
  assert.match(tagsPageSource, /tagDisplayGroupRank\(a\.tag\) - tagDisplayGroupRank\(b\.tag\)/);
  assert.match(tagsPageSource, /return rankDelta \|\| a\.index - b\.index;/);
  assert.doesNotMatch(tagsPageSource, /sourceLabel\(tag\.source,\s*tag\)/);
  assert.match(tagsPageSource, /return "自动生成"/);
  assert.doesNotMatch(tagsPageSource, /return "爬虫"/);
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

test("admin tag card delete action appears before edit action", () => {
  const actionsStart = tagsPageSource.indexOf('className="admin-tag-card__footer-actions"');
  const deleteIndex = tagsPageSource.indexOf('className="admin-tag-card__delete"', actionsStart);
  const editIndex = tagsPageSource.indexOf('className="admin-tag-card__edit"', actionsStart);
  const actionsEnd = tagsPageSource.indexOf("</div>", editIndex);
  const actionsSource = tagsPageSource.slice(actionsStart, actionsEnd);

  assert.ok(actionsStart >= 0, "tag card footer actions should exist");
  assert.ok(deleteIndex > actionsStart, "delete action should be inside tag card actions");
  assert.ok(editIndex > deleteIndex, "edit action should stay to the right of delete action");
  assert.doesNotMatch(actionsSource, /<Trash2\b|<Pencil\b/);
  assert.doesNotMatch(tagsPageSource, /import \{[^}]*\bPencil\b/);
});

test("admin tag dialogs use the lightweight modal style", () => {
  assert.match(
    tagsPageSource,
    /modalClassName="admin-modal--delete-confirm admin-modal--tag-dialog admin-modal--tag-delete-confirm"/
  );
  assert.match(
    tagsPageSource,
    /title="新增标签"[\s\S]*?className="admin-modal--tag-rules admin-modal--tag-dialog"/
  );
  assert.match(tagsPageSource, /className="admin-modal--tag-rules admin-modal--tag-dialog"/);
  assert.match(tagsPageSource, /restoreFocus=\{false\}/);
  assert.match(adminCss, /\.admin-modal--tag-dialog\s*\{[^}]*border\s*:\s*0/s);
  assert.match(adminCss, /\.admin-modal--tag-dialog \.admin-modal__header\s*\{[^}]*border-bottom\s*:\s*0/s);
  assert.match(adminCss, /\.admin-modal--tag-dialog \.admin-modal__footer\s*\{[^}]*border-top\s*:\s*0/s);
  assert.match(adminCss, /\.admin-modal--tag-delete-confirm \.admin-confirm\s*\{[^}]*display\s*:\s*block/s);
});

test("admin tag edit dialog shows aliases as pills outside the input", () => {
  const editModalStart = tagsPageSource.indexOf("function EditTagModal");
  const editModalSource = tagsPageSource.slice(editModalStart);
  assert.ok(editModalStart >= 0, "EditTagModal should exist");
  assert.match(
    editModalSource,
    /const \[aliases, setAliases\] = useState\(\(\) => editTagAliases\(tag\)\)/
  );
  assert.match(editModalSource, /const \[aliasDraft, setAliasDraft\] = useState\(""\)/);
  assert.match(editModalSource, /const aliasAdditions = pendingAliasAdditions\(aliasDraft, tag\.label, aliases\);/);
  assert.match(editModalSource, /const duplicateAliases = duplicateAliasInputs\(aliasDraft, tag\.label, aliases\);/);
  assert.match(editModalSource, /title=\{tag\.label\}/);
  assert.match(editModalSource, /\{saving \? "保存中\.\.\." : "保存"\}/);
  assert.match(editModalSource, /className="admin-tag-alias-list"/);
  assert.match(editModalSource, /className="admin-tag-alias-pill"/);
  assert.match(editModalSource, /aria-label=\{`移除别名 \$\{alias\}`\}/);
  assert.match(editModalSource, /value=\{aliasDraft\}/);
  assert.match(editModalSource, /当前标签已存在/);
  assert.doesNotMatch(editModalSource, /已存在：\{duplicateAliases\.join\("、"\)\}/);
  assert.match(editModalSource, /disabled=\{saving \|\| aliasAdditions\.length === 0\}/);
  assert.match(editModalSource, /await api\.updateTag\(tag\.id, aliases\);/);
  assert.doesNotMatch(editModalSource, /useState\(\(tag\.aliases \?\? \[\]\)\.join\(", "\)\)/);
  assert.doesNotMatch(editModalSource, /value=\{aliases\}/);
  assert.doesNotMatch(editModalSource, /admin-form__help/);
  assert.match(adminCss, /\.admin-tag-alias-list\s*\{[^}]*display\s*:\s*flex/s);
  assert.match(adminCss, /\.admin-tag-alias-list\s*\{[^}]*gap\s*:\s*8px/s);
  assert.match(adminCss, /\.admin-tag-alias-pill\s*\{[^}]*background\s*:\s*transparent/s);
  assert.match(adminCss, /\.admin-tag-alias-pill\s*\{[^}]*border\s*:\s*1px solid var\(--border-subtle\)/s);
  assert.match(adminCss, /\.admin-tag-alias-warning\s*\{[^}]*color\s*:\s*var\(--danger\)/s);
  assert.match(tagsPageSource, /function editTagAliases\(tag: api\.AdminTag\): string\[\]/);
  assert.match(tagsPageSource, /\[\.\.\.\(tag\.matchRules\?\.avCodePrefixes \?\? \[\]\), \.\.\.\(tag\.aliases \?\? \[\]\)\]/);
});

test("admin videos render tag assignment source and evidence", () => {
  assert.match(apiSource, /tagSources\?: Record<string, string>/);
  assert.match(apiSource, /tagEvidence\?: Record<string, string>/);
  assert.match(videosPageSource, /data-source=\{v\.tagSources\?\.\[t\]/);
  assert.match(videosPageSource, /tagAssignmentSourceLabel/);
  assert.match(videosPageSource, /tagAssignmentTitle/);
  assert.match(videosPageSource, /video\.tagEvidence\?\.\[tag\.label\]/);
});
