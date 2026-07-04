import { useEffect, useMemo, useState } from "react";
import { Film, RefreshCw, Search, Trash2 } from "lucide-react";
import * as api from "./api";
import { useToast } from "./ToastContext";
import { ConfirmModal } from "./ConfirmModal";
import { Modal } from "./Modal";

const DESKTOP_TAGS_PAGE_SIZE = 25;
const MOBILE_TAGS_PAGE_SIZE = 8;
const TAGS_MOBILE_QUERY = "(max-width: 640px)";
const TAG_SOURCE_FILTERS = ["builtin", "user", "generated"];
const TAG_DISPLAY_GROUP_ORDER: Record<string, number> = {
  builtin: 0,
  user: 1,
  crawler: 2,
  av: 3,
};

type DeleteConfirmState =
  | { kind: "single"; tag: api.AdminTag }
  | { kind: "bulk"; ids: number[] }
  | null;

export function TagsPage() {
  const [tags, setTags] = useState<api.AdminTag[]>([]);
  const [label, setLabel] = useState("");
  const [aliases, setAliases] = useState("");
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [saving, setSaving] = useState(false);
  const [deletingId, setDeletingId] = useState<number | null>(null);
  const [deleteConfirm, setDeleteConfirm] = useState<DeleteConfirmState>(null);
  const [searchQuery, setSearchQuery] = useState("");
  const [filterSource, setFilterSource] = useState<string>("all");
  const [selectMode, setSelectMode] = useState(false);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [bulkDeleting, setBulkDeleting] = useState(false);
  const [editingTag, setEditingTag] = useState<api.AdminTag | null>(null);
  const [createModalOpen, setCreateModalOpen] = useState(false);
  const pageSize = useTagsPageSize();
  const [page, setPage] = useState(1);
  const { show } = useToast();

  async function refresh() {
    setLoading(true);
    setLoadError("");
    try {
      setTags(await api.listTags());
    } catch (e) {
      const message = e instanceof Error ? e.message : "加载标签失败";
      setLoadError(message);
      show(message, "error");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  async function handleCreate() {
    const cleanLabel = label.trim();
    if (!cleanLabel) return;
    setSaving(true);
    try {
      const r = await api.createTag(cleanLabel, splitList(aliases));
      show(`已添加标签「${r.label}」`, "success");
      setLabel("");
      setAliases("");
      setCreateModalOpen(false);
      await refresh();
    } catch (e) {
      show(e instanceof Error ? e.message : "添加标签失败", "error");
    } finally {
      setSaving(false);
    }
  }

  function handleDelete(tag: api.AdminTag) {
    setDeleteConfirm({ kind: "single", tag });
  }

  function openCreateModal() {
    setLabel("");
    setAliases("");
    setCreateModalOpen(true);
  }

  function toggleSelectMode() {
    setSelectMode((m) => !m);
    setSelected(new Set());
  }

  function toggleSelect(id: number) {
    setSelected((prev) => {
      const next = new Set(prev);
      next.has(id) ? next.delete(id) : next.add(id);
      return next;
    });
  }

  async function handleBulkDelete() {
    const ids = [...selected];
    if (ids.length === 0) return;
    setDeleteConfirm({ kind: "bulk", ids });
  }

  async function confirmDelete() {
    if (!deleteConfirm) return;

    if (deleteConfirm.kind === "single") {
      const tag = deleteConfirm.tag;
      setDeletingId(tag.id);
      try {
        const r = await api.deleteTag(tag.id);
        show(`已删除标签，并从 ${r.removedVideos} 个视频移除`, "success");
        setDeleteConfirm(null);
        await refresh();
      } catch (e) {
        show(e instanceof Error ? e.message : "删除标签失败", "error");
      } finally {
        setDeletingId(null);
      }
      return;
    }

    const ids = deleteConfirm.ids;
    setBulkDeleting(true);
    try {
      let success = 0;
      for (const id of ids) {
        try {
          await api.deleteTag(id);
          success++;
        } catch {
          // Keep deleting the rest of the selected tags; report aggregate failure below.
        }
      }
      const failed = ids.length - success;
      show(
        failed ? `批量删除完成，成功 ${success} / ${ids.length} 个，失败 ${failed} 个` : `已删除 ${success} 个标签`,
        failed ? (success > 0 ? "info" : "error") : "success"
      );
      setSelected(new Set());
      setSelectMode(false);
      setDeleteConfirm(null);
      await refresh();
    } finally {
      setBulkDeleting(false);
    }
  }

  const stats = useMemo(() => {
    const sourceCounts: Record<string, number> = {};
    let total = 0;

    tags.forEach((t) => {
      if (!isSupportedTag(t)) return;
      total++;
      const key = tagSourceKey(t);
      sourceCounts[key] = (sourceCounts[key] ?? 0) + 1;
    });

    return {
      total,
      sourceCounts,
    };
  }, [tags]);

  const filteredTags = useMemo(() => {
    const query = searchQuery.trim().toLowerCase();
    const matches = tags.filter((t) => {
      if (!isSupportedTag(t)) return false;
      const matchesSearch =
        !query ||
        t.label.toLowerCase().includes(query) ||
        (t.aliases ?? []).some((a) => a.toLowerCase().includes(query));
      const matchesSource = filterSource === "all" || tagSourceKey(t) === filterSource;
      return matchesSearch && matchesSource;
    });

    if (filterSource !== "all") return matches;

    return matches
      .map((tag, index) => ({ tag, index }))
      .sort((a, b) => {
        const rankDelta = tagDisplayGroupRank(a.tag) - tagDisplayGroupRank(b.tag);
        return rankDelta || a.index - b.index;
      })
      .map(({ tag }) => tag);
  }, [tags, searchQuery, filterSource]);

  const totalPages = Math.max(1, Math.ceil(filteredTags.length / pageSize));
  const currentPage = Math.min(page, totalPages);
  const pageStartIndex = (currentPage - 1) * pageSize;
  const pageEndIndex = pageStartIndex + pageSize;
  const pagedTags = useMemo(
    () => filteredTags.slice(pageStartIndex, pageEndIndex),
    [filteredTags, pageStartIndex, pageEndIndex]
  );
  const pageStart = filteredTags.length === 0 ? 0 : pageStartIndex + 1;
  const pageEnd = Math.min(filteredTags.length, pageEndIndex);

  useEffect(() => {
    setPage(1);
  }, [searchQuery, filterSource, pageSize]);

  useEffect(() => {
    setPage((p) => Math.min(Math.max(1, p), totalPages));
  }, [totalPages]);

  const deletablePageTags = useMemo(
    () => pagedTags,
    [pagedTags]
  );
  const allSelected =
    deletablePageTags.length > 0 && deletablePageTags.every((t) => selected.has(t.id));

  function selectPageTags() {
    setSelected((prev) => {
      const next = new Set(prev);
      deletablePageTags.forEach((t) => next.add(t.id));
      return next;
    });
  }

  return (
    <section className={`admin-tags-page${selectMode ? " has-bulk-actions" : ""}`}>
      <div className="admin-tags-layout">
        <div className="admin-tags-main">
          <div className="admin-tags-toolbar">
            <div className="admin-tags-search">
              <Search className="admin-tags-search__icon" size={14} />
              <input
                aria-label="搜索标签名或别名"
                type="text"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="搜索标签名或别名..."
              />
            </div>

            <aside className="admin-tags-filter-panel" aria-label="标签分类">
              <div className="admin-tags-filter-tabs">
                <button
                  type="button"
                  className={`admin-tags-filter-tab ${filterSource === "all" ? "is-active" : ""}`}
                  onClick={() => setFilterSource("all")}
                  aria-label="全部"
                >
                  <span className="admin-tags-filter-tab__text">全部</span>
                </button>
                {TAG_SOURCE_FILTERS.filter((source) => (stats.sourceCounts[source] ?? 0) > 0).map((source) => {
                  const label = sourceLabel(source);
                  return (
                    <button
                      key={source}
                      type="button"
                      className={`admin-tags-filter-tab ${filterSource === source ? "is-active" : ""}`}
                      onClick={() => setFilterSource(source)}
                      aria-label={label}
                    >
                      <span className="admin-tags-filter-tab__text">{label}</span>
                    </button>
                  );
                })}
              </div>
            </aside>

            <div className="admin-tags-toolbar-actions">
              <button
                type="button"
                className="admin-btn"
                onClick={openCreateModal}
              >
                新增标签
              </button>
              <button
                type="button"
                className={`admin-btn ${selectMode ? "is-primary" : ""}`}
                onClick={toggleSelectMode}
              >
                {selectMode ? "退出批量" : "批量删除"}
              </button>
            </div>
          </div>

          <div className="admin-tags-board">
            <div className="admin-tags-cards">
              {loading ? (
                <div className="admin-loading-state">
                  <RefreshCw size={20} className="admin-spin" />
                  <span>加载中...</span>
                </div>
              ) : loadError ? (
                <div className="admin-error-state">
                  <strong>标签加载失败</strong>
                  <span>{loadError}</span>
                  <button type="button" className="admin-btn" onClick={refresh}>
                    <RefreshCw size={13} /> 重试
                  </button>
                </div>
              ) : filteredTags.length === 0 ? (
                <div className="admin-card admin-empty">
                  没有找到匹配的标签。
                </div>
              ) : (
                <>
                  <div className="admin-tags-grid">
                    {pagedTags.map((tag) => {
                      const selectable = selectMode;
                      const isSelected = selected.has(tag.id);
                      const cardClass = `admin-tag-card${selectable ? " is-selectable" : ""}${
                        selectable && isSelected ? " is-selected" : ""
                      }`;
                      const cardContent = (
                        <>
                          <div className="admin-tag-card__head">
                            <span className="admin-tag-card__title">{tag.label}</span>
                            <span className="admin-tag-card__source-badge" data-source={tagSourceKey(tag)}>
                              {tagCardSourceLabel(tag)}
                            </span>
                          </div>

                          <div className="admin-tag-card__footer">
                            <span className="admin-tag-card__count">
                              <Film size={13} />
                              <strong>{tag.count}</strong> 视频
                            </span>
                            <div className="admin-tag-card__footer-actions">
                              {!selectMode && (
                                <button
                                  type="button"
                                  className="admin-tag-card__delete"
                                  onClick={() => handleDelete(tag)}
                                  disabled={deletingId === tag.id}
                                  aria-label={`删除标签 ${tag.label}`}
                                >
                                  <span>{deletingId === tag.id ? "删除中" : "删除"}</span>
                                </button>
                              )}
                              {!selectMode && (
                                <button
                                  type="button"
                                  className="admin-tag-card__edit"
                                  onClick={() => setEditingTag(tag)}
                                  aria-label={`编辑标签 ${tag.label}`}
                                >
                                  <span>编辑</span>
                                </button>
                              )}
                            </div>
                          </div>
                        </>
                      );
                      return selectable ? (
                        <button
                          key={tag.id}
                          type="button"
                          className={cardClass}
                          onClick={() => toggleSelect(tag.id)}
                          aria-pressed={isSelected}
                          aria-label={`${isSelected ? "取消选中" : "选中"}标签 ${tag.label}`}
                        >
                          {cardContent}
                        </button>
                      ) : (
                        <div key={tag.id} className={cardClass}>
                          {cardContent}
                        </div>
                      );
                    })}
                  </div>

                  {totalPages > 1 && (
                    <div className="admin-table-pagination admin-tags-pagination">
                      <button
                        type="button"
                        className="admin-btn"
                        onClick={() => setPage(1)}
                        disabled={currentPage <= 1}
                      >
                        首页
                      </button>
                      <button
                        type="button"
                        className="admin-btn"
                        onClick={() => setPage((p) => Math.max(1, p - 1))}
                        disabled={currentPage <= 1}
                      >
                        上一页
                      </button>
                      <span className="admin-table-pagination__info">
                        第 {currentPage} / {totalPages} 页，显示 {pageStart}-{pageEnd} / {filteredTags.length}，每页 {pageSize} 个
                      </span>
                      <button
                        type="button"
                        className="admin-btn"
                        onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                        disabled={currentPage >= totalPages}
                      >
                        下一页
                      </button>
                      <button
                        type="button"
                        className="admin-btn"
                        onClick={() => setPage(totalPages)}
                        disabled={currentPage >= totalPages}
                      >
                        末页
                      </button>
                    </div>
                  )}
                </>
              )}
            </div>
          </div>
        </div>
      </div>
      {selectMode && (
        <div className="admin-tags-bulk-toolbar" role="region" aria-label="标签批量操作">
          <div className="admin-tags-bulk-actions">
            <span className="admin-tags-bulk-actions__count">已选择 {selected.size} 项</span>
            <button
              type="button"
              className="admin-btn admin-tags-bulk-actions__btn admin-tags-bulk-actions__select-page"
              onClick={selectPageTags}
              disabled={deletablePageTags.length === 0 || allSelected}
            >
              全选本页
            </button>
            <button
              type="button"
              className="admin-btn admin-tags-bulk-actions__btn"
              onClick={() => setSelected(new Set())}
              disabled={selected.size === 0}
            >
              取消选中
            </button>
            <button
              type="button"
              className="admin-btn is-danger admin-tags-bulk-actions__btn"
              onClick={handleBulkDelete}
              disabled={selected.size === 0 || bulkDeleting}
            >
              <Trash2 size={13} /> {bulkDeleting ? "删除中..." : "删除选中"}
            </button>
          </div>
        </div>
      )}
      <Modal
        open={createModalOpen}
        title="新增标签"
        className="admin-modal--tag-rules admin-modal--tag-dialog"
        onClose={() => {
          if (!saving) setCreateModalOpen(false);
        }}
        footer={
          <>
            <button
              type="button"
              className="admin-btn"
              onClick={() => setCreateModalOpen(false)}
              disabled={saving}
            >
              取消
            </button>
            <button
              type="submit"
              form="admin-create-tag-form"
              className="admin-btn is-primary"
              disabled={saving || !label.trim()}
            >
              {saving ? "添加中..." : "添加标签"}
            </button>
          </>
        }
      >
        <form
          id="admin-create-tag-form"
          className="admin-form admin-tag-rule-form"
          onSubmit={(e) => {
            e.preventDefault();
            handleCreate();
          }}
        >
          <div className="admin-form__row">
            <label htmlFor="admin-tag-label">标签名</label>
            <input
              id="admin-tag-label"
              value={label}
              onChange={(e) => setLabel(e.target.value)}
              placeholder="例如：清纯"
            />
          </div>
          <div className="admin-form__row">
            <label htmlFor="admin-tag-aliases">别名</label>
            <input
              id="admin-tag-aliases"
              value={aliases}
              onChange={(e) => setAliases(e.target.value)}
              placeholder="逗号分隔，例如：纯欲, 清新"
            />
          </div>
        </form>
      </Modal>
      {editingTag && (
        <EditTagModal
          tag={editingTag}
          onClose={() => setEditingTag(null)}
          onSaved={async () => {
            setEditingTag(null);
            await refresh();
          }}
        />
      )}
      <ConfirmModal
        open={!!deleteConfirm}
        title={deleteConfirm?.kind === "bulk" ? "删除选中标签" : "删除标签"}
        message={
          deleteConfirm?.kind === "bulk"
            ? `确定要删除选中的 ${deleteConfirm.ids.length} 个标签吗？`
            : `确定要删除标签「${deleteConfirm?.tag.label ?? ""}」吗？`
        }
        confirmText="确认删除"
        danger
        centerMessage
        modalClassName="admin-modal--delete-confirm admin-modal--tag-dialog admin-modal--tag-delete-confirm"
        loading={deletingId !== null || bulkDeleting}
        restoreFocus={false}
        onCancel={() => {
          if (deletingId === null && !bulkDeleting) setDeleteConfirm(null);
        }}
        onConfirm={confirmDelete}
      />
    </section>
  );
}

function EditTagModal({
  tag,
  onClose,
  onSaved,
}: {
  tag: api.AdminTag;
  onClose: () => void;
  onSaved: () => void | Promise<void>;
}) {
  const [aliases, setAliases] = useState(() => editTagAliases(tag));
  const [aliasDraft, setAliasDraft] = useState("");
  const [saving, setSaving] = useState(false);
  const { show } = useToast();
  const aliasAdditions = pendingAliasAdditions(aliasDraft, tag.label, aliases);
  const duplicateAliases = duplicateAliasInputs(aliasDraft, tag.label, aliases);

  async function save() {
    setSaving(true);
    try {
      await api.updateTag(tag.id, aliases);
      show("标签已保存", "success");
      await onSaved();
    } catch (e) {
      show(e instanceof Error ? e.message : "保存标签失败", "error");
    } finally {
      setSaving(false);
    }
  }

  function addAliases(raw: string) {
    const additions = pendingAliasAdditions(raw, tag.label, aliases);
    if (additions.length === 0) return;
    setAliases((current) => normalizeAliasList([...current, ...additions], tag.label));
    setAliasDraft("");
  }

  function removeAlias(alias: string) {
    setAliases((current) => current.filter((item) => item !== alias));
  }

  return (
    <Modal
      open
      title={tag.label}
      className="admin-modal--tag-rules admin-modal--tag-dialog"
      onClose={onClose}
      restoreFocus={false}
      footer={
        <>
          <button type="button" className="admin-btn" onClick={onClose} disabled={saving}>
            取消
          </button>
          <button type="button" className="admin-btn is-primary" onClick={save} disabled={saving}>
            {saving ? "保存中..." : "保存"}
          </button>
        </>
      }
    >
      <div className="admin-form admin-tag-rule-form">
        <div className="admin-form__row">
          <span className="admin-form__label" id="admin-tag-aliases-current-label">
            已有别名
          </span>
          <div className="admin-tag-alias-list" aria-labelledby="admin-tag-aliases-current-label">
            {aliases.length > 0 ? (
              aliases.map((alias) => (
                <span key={alias} className="admin-tag-alias-pill">
                  <span>{alias}</span>
                  <button
                    type="button"
                    onClick={() => removeAlias(alias)}
                    disabled={saving}
                    aria-label={`移除别名 ${alias}`}
                  >
                    移除
                  </button>
                </span>
              ))
            ) : (
              <span className="admin-tag-alias-empty">暂无别名</span>
            )}
          </div>
        </div>
        <div className="admin-form__row">
          <label htmlFor="admin-tag-rule-aliases">新增别名</label>
          <div className="admin-tag-alias-input-row">
            <input
              id="admin-tag-rule-aliases"
              value={aliasDraft}
              onChange={(e) => setAliasDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key !== "Enter") return;
                e.preventDefault();
                addAliases(aliasDraft);
              }}
              placeholder="输入别名"
              aria-describedby={duplicateAliases.length > 0 ? "admin-tag-alias-warning" : undefined}
            />
            <button
              type="button"
              className="admin-btn"
              onClick={() => addAliases(aliasDraft)}
              disabled={saving || aliasAdditions.length === 0}
            >
              添加
            </button>
          </div>
          {duplicateAliases.length > 0 && (
            <span className="admin-tag-alias-warning" id="admin-tag-alias-warning">
              当前标签已存在
            </span>
          )}
        </div>
      </div>
    </Modal>
  );
}

function useTagsPageSize() {
  const [pageSize, setPageSize] = useState(() =>
    window.matchMedia(TAGS_MOBILE_QUERY).matches
      ? MOBILE_TAGS_PAGE_SIZE
      : DESKTOP_TAGS_PAGE_SIZE
  );

  useEffect(() => {
    const media = window.matchMedia(TAGS_MOBILE_QUERY);
    const update = () => {
      setPageSize(media.matches ? MOBILE_TAGS_PAGE_SIZE : DESKTOP_TAGS_PAGE_SIZE);
    };
    update();
    media.addEventListener("change", update);
    return () => media.removeEventListener("change", update);
  }, []);

  return pageSize;
}

function splitList(s: string): string[] {
  return s
    .split(/[,，、\s]+/)
    .map((x) => x.trim())
    .filter(Boolean);
}

function normalizeAliasList(aliases: string[], label: string): string[] {
  const labelKey = label.trim().toLowerCase();
  const seen = new Set<string>();
  const out: string[] = [];
  for (const alias of aliases) {
    const clean = alias.trim();
    const key = clean.toLowerCase();
    if (!clean || key === labelKey || seen.has(key)) continue;
    seen.add(key);
    out.push(clean);
  }
  return out;
}

function editTagAliases(tag: api.AdminTag): string[] {
  if (tag.label.trim().toUpperCase() === "AV") {
    return normalizeAliasList(
      [...(tag.matchRules?.avCodePrefixes ?? []), ...(tag.aliases ?? [])],
      tag.label
    );
  }
  return normalizeAliasList(tag.aliases ?? [], tag.label);
}

function pendingAliasAdditions(raw: string, label: string, existingAliases: string[]): string[] {
  const existingKeys = new Set(existingAliases.map((alias) => alias.trim().toLowerCase()));
  return normalizeAliasList(splitList(raw), label).filter(
    (alias) => !existingKeys.has(alias.toLowerCase())
  );
}

function duplicateAliasInputs(raw: string, label: string, existingAliases: string[]): string[] {
  const labelKey = label.trim().toLowerCase();
  const existingKeys = new Set(existingAliases.map((alias) => alias.trim().toLowerCase()));
  const inputKeys = new Set<string>();
  const duplicateKeys = new Set<string>();
  const duplicates: string[] = [];

  for (const alias of splitList(raw)) {
    const clean = alias.trim();
    const key = clean.toLowerCase();
    if (!clean) continue;
    if (key === labelKey || existingKeys.has(key) || inputKeys.has(key)) {
      if (!duplicateKeys.has(key)) {
        duplicateKeys.add(key);
        duplicates.push(clean);
      }
      continue;
    }
    inputKeys.add(key);
  }

  return duplicates;
}

function sourceLabel(source: string): string {
  if (source === "crawler" || source === "generated") return "自动生成";
  if (source === "builtin") return "内置";
  if (source === "user") return "自定义";
  return source || "未知";
}

function tagCardSourceLabel(tag: api.AdminTag): string {
  if (tag.crawlerOwned || tag.source === "crawler") return "爬虫脚本";
  if (tag.source === "generated") return "AV";
  return sourceLabel(tag.source);
}

function tagDisplayGroupKey(tag: api.AdminTag): string {
  if (tag.source === "builtin" || tag.source === "user") return tag.source;
  if (tag.crawlerOwned || tag.source === "crawler") return "crawler";
  if (tag.source === "generated") return "av";
  return tag.source || "";
}

function tagDisplayGroupRank(tag: api.AdminTag): number {
  return TAG_DISPLAY_GROUP_ORDER[tagDisplayGroupKey(tag)] ?? 99;
}

function tagSourceKey(tag: api.AdminTag): string {
  return tag.crawlerOwned || tag.source === "generated" ? "generated" : tag.source;
}

function isSupportedTag(tag: api.AdminTag): boolean {
  return tag.source === "builtin" || tag.source === "user" || tag.source === "generated" || tag.crawlerOwned === true;
}
