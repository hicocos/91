import { useEffect, useMemo, useState } from "react";
import { Film, Pencil, Plus, RefreshCw, Search, Settings2, Trash2 } from "lucide-react";
import * as api from "./api";
import { useToast } from "./ToastContext";
import { ConfirmModal } from "./ConfirmModal";
import { Modal } from "./Modal";

const DESKTOP_TAGS_PAGE_SIZE = 25;
const MOBILE_TAGS_PAGE_SIZE = 8;
const TAGS_MOBILE_QUERY = "(max-width: 640px)";
const TAG_SOURCE_FILTERS = ["builtin", "user", "generated"];

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
  const [retagConfirmOpen, setRetagConfirmOpen] = useState(false);
  const [jobStatus, setJobStatus] = useState<api.TagJobStatus | null>(null);
  const [jobStarting, setJobStarting] = useState<"retag" | null>(null);
  const [autoGenerateTagsEnabled, setAutoGenerateTagsEnabled] = useState(true);
  const [autoGenerateTagsSaving, setAutoGenerateTagsSaving] = useState(false);
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
    void api.getTagJobStatus().then(setJobStatus).catch(() => undefined);
    void api
      .getSettings()
      .then((settings) => {
        if (typeof settings.autoGenerateTagsEnabled === "boolean") {
          setAutoGenerateTagsEnabled(settings.autoGenerateTagsEnabled);
        }
      })
      .catch(() => undefined);
  }, []);

  useEffect(() => {
    if (!jobStatus?.running) return;
    let active = true;
    let timer = 0;
    const poll = async () => {
      try {
        const next = await api.getTagJobStatus();
        if (!active) return;
        setJobStatus(next);
        if (next.running) {
          timer = window.setTimeout(poll, 1500);
          return;
        }
        show(
          next.state === "completed"
            ? `标签整理完成，共处理 ${next.processed} 个视频`
            : next.lastError || "标签任务未完成",
          next.state === "completed" ? "success" : "error"
        );
        void refresh();
      } catch {
        if (active) timer = window.setTimeout(poll, 2000);
      }
    };
    timer = window.setTimeout(poll, 1000);
    return () => {
      active = false;
      window.clearTimeout(timer);
    };
  }, [jobStatus?.running]);

  async function startRetag() {
    setJobStarting("retag");
    try {
      await api.startTagRetag();
      setRetagConfirmOpen(false);
      setJobStatus(await api.getTagJobStatus());
      show("已开始后台重新整理所有标签", "info");
    } catch (e) {
      show(e instanceof Error ? e.message : "启动重算失败", "error");
    } finally {
      setJobStarting(null);
    }
  }

  async function handleCreate() {
    const cleanLabel = label.trim();
    if (!cleanLabel) return;
    setSaving(true);
    try {
      const r = await api.createTag(cleanLabel, splitList(aliases));
      show(`已添加标签，自动匹配 ${r.classified} 个视频`, "success");
      setLabel("");
      setAliases("");
      await refresh();
    } catch (e) {
      show(e instanceof Error ? e.message : "添加标签失败", "error");
    } finally {
      setSaving(false);
    }
  }

  async function toggleAutoGenerateTags() {
    const next = !autoGenerateTagsEnabled;
    const previous = autoGenerateTagsEnabled;
    setAutoGenerateTagsEnabled(next);
    setAutoGenerateTagsSaving(true);
    try {
      const settings = await api.updateSettings({ autoGenerateTagsEnabled: next });
      if (typeof settings.autoGenerateTagsEnabled === "boolean") {
        setAutoGenerateTagsEnabled(settings.autoGenerateTagsEnabled);
      }
      show(next ? "已开启自动生成标签" : "已关闭自动生成标签", "success");
    } catch (e) {
      setAutoGenerateTagsEnabled(previous);
      show(e instanceof Error ? e.message : "保存设置失败", "error");
    } finally {
      setAutoGenerateTagsSaving(false);
    }
  }

  function handleDelete(tag: api.AdminTag) {
    setDeleteConfirm({ kind: "single", tag });
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

    tags.forEach((t) => {
      sourceCounts[t.source] = (sourceCounts[t.source] ?? 0) + 1;
    });

    return {
      sourceCounts,
    };
  }, [tags]);

  const filteredTags = useMemo(() => {
    return tags.filter((t) => {
      const query = searchQuery.trim().toLowerCase();
      const matchesSearch =
        !query ||
        t.label.toLowerCase().includes(query) ||
        (t.aliases ?? []).some((a) => a.toLowerCase().includes(query));
      const matchesSource = filterSource === "all" || t.source === filterSource;
      return matchesSearch && matchesSource;
    });
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
      <header className="admin-page__header">
        <h1 className="admin-page__title">标签管理</h1>
      </header>

      <div className="admin-tags-layout">
        {/* 左栏：创建与策略 */}
        <div>
          <div className="admin-card">
            <div className="admin-card__title">
              <Plus size={15} /> 新增标签
            </div>
            <form
              className="admin-form"
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
              <button
                type="submit"
                className="admin-btn is-primary"
                disabled={saving || !label.trim()}
              >
                <Plus size={13} /> {saving ? "添加中..." : "添加并自动匹配"}
              </button>
            </form>
          </div>

          <div className="admin-card">
            <div className="admin-card__title">
              <Settings2 size={15} /> 标签策略
            </div>
            <div
              className={`admin-tag-setting-toggle${autoGenerateTagsEnabled ? " is-on" : ""}${
                autoGenerateTagsSaving ? " is-saving" : ""
              }`}
            >
              <span className="admin-tag-setting-toggle__title">自动生成标签</span>
              <button
                type="button"
                className="admin-tag-setting-toggle__switch"
                onClick={toggleAutoGenerateTags}
                disabled={autoGenerateTagsSaving}
                role="switch"
                aria-checked={autoGenerateTagsEnabled}
                aria-label="自动生成标签"
              >
                <span className="admin-tag-setting-toggle__switch-text">
                  {autoGenerateTagsEnabled ? "ON" : "OFF"}
                </span>
              </button>
            </div>
          </div>
        </div>

        {/* 右栏：看板网格与搜索栏 */}
        <div>
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

            <div className="admin-tags-filter-tabs">
              <button
                type="button"
                className={`admin-tags-filter-tab ${filterSource === "all" ? "is-active" : ""}`}
                onClick={() => setFilterSource("all")}
              >
                全部 ({tags.length})
              </button>
              {TAG_SOURCE_FILTERS.filter((source) => (stats.sourceCounts[source] ?? 0) > 0).map((source) => (
                <button
                  key={source}
                  type="button"
                  className={`admin-tags-filter-tab ${filterSource === source ? "is-active" : ""}`}
                  onClick={() => setFilterSource(source)}
                >
                  {sourceLabel(source)} ({stats.sourceCounts[source]})
                </button>
              ))}
            </div>

            <div className="admin-tags-toolbar-actions">
              {jobStatus?.running && (
                <span className="admin-tags-job-status">
                  整理标签 {jobStatus.processed}/{jobStatus.total || "?"}
                </span>
              )}
              <button
                type="button"
                className="admin-btn"
                onClick={() => setRetagConfirmOpen(true)}
                disabled={!!jobStatus?.running || jobStarting !== null}
              >
                <RefreshCw size={13} /> 重新整理所有标签
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
                        <span className="admin-tag-card__source-badge" data-source={tag.source}>
                          {sourceLabel(tag.source, tag)}
                        </span>
                      </div>

                      {tag.aliases && tag.aliases.length > 0 && (
                        <div className="admin-tag-card__aliases">
                          {tag.aliases.map((alias) => (
                            <span key={alias} className="admin-tag-card__alias-pill">
                              {alias}
                            </span>
                          ))}
                        </div>
                      )}

                      <div className="admin-tag-card__footer">
                        <span className="admin-tag-card__count">
                          <Film size={13} />
                          <strong>{tag.count}</strong> 视频
                        </span>
                        <div className="admin-tag-card__footer-actions">
                          <span className="admin-tag-card__id">#{tag.id}</span>
                          {!selectMode && (
                            <button
                              type="button"
                              className="admin-tag-card__edit"
                              onClick={() => setEditingTag(tag)}
                              aria-label={`编辑标签 ${tag.label}`}
                            >
                              <Pencil size={11} />
                              <span>编辑</span>
                            </button>
                          )}
                          {!selectMode && (
                            <button
                              type="button"
                              className="admin-tag-card__delete"
                              onClick={() => handleDelete(tag)}
                              disabled={deletingId === tag.id}
                              aria-label={`删除标签 ${tag.label}`}
                            >
                              <Trash2 size={11} />
                              <span>{deletingId === tag.id ? "删除中" : "删除"}</span>
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
        open={retagConfirmOpen}
        title="重新整理所有标签"
        message="将清理普通自动生成标签、自动匹配关系和标签墓碑，再按当前设置重新整理标签。"
        details={[
          "会保留自定义、内置和爬虫脚本等非普通自动生成标签；内置标签会在清空墓碑后恢复。",
          "如果“自动生成标签”已开启，会重新执行自动匹配和番号系列维护；关闭时只执行清理和非自动整理。",
          "任务在后台分批执行，运行期间不能同时启动另一个标签整理任务。",
        ]}
        confirmText="开始整理"
        loading={jobStarting === "retag"}
        onCancel={() => {
          if (jobStarting !== "retag") setRetagConfirmOpen(false);
        }}
        onConfirm={startRetag}
      />
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
        modalClassName="admin-modal--delete-confirm"
        loading={deletingId !== null || bulkDeleting}
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
  const [aliases, setAliases] = useState((tag.aliases ?? []).join(", "));
  const [keywords, setKeywords] = useState((tag.matchRules?.keywords ?? []).join(", "));
  const [words, setWords] = useState((tag.matchRules?.words ?? []).join(", "));
  const [excludes, setExcludes] = useState((tag.matchRules?.excludes ?? []).join(", "));
  const [matchAvCode, setMatchAvCode] = useState(!!tag.matchRules?.matchAvCode);
  const [saving, setSaving] = useState(false);
  const { show } = useToast();

  async function save() {
    setSaving(true);
    try {
      const result = await api.updateTag(tag.id, splitList(aliases), {
        keywords: splitList(keywords),
        words: splitList(words),
        excludes: splitList(excludes),
        matchAvCode,
      });
      show(`规则已保存，新匹配 ${result.classified} 个视频`, "success");
      await onSaved();
    } catch (e) {
      show(e instanceof Error ? e.message : "保存标签规则失败", "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <Modal
      open
      title={`编辑标签：${tag.label}`}
      className="admin-modal--tag-rules"
      onClose={onClose}
      footer={
        <>
          <button type="button" className="admin-btn" onClick={onClose} disabled={saving}>
            取消
          </button>
          <button type="button" className="admin-btn is-primary" onClick={save} disabled={saving}>
            {saving ? "保存中..." : "保存规则"}
          </button>
        </>
      }
    >
      <div className="admin-form admin-tag-rule-form">
        <div className="admin-form__row">
          <label htmlFor="admin-tag-rule-aliases">别名</label>
          <input
            id="admin-tag-rule-aliases"
            value={aliases}
            onChange={(e) => setAliases(e.target.value)}
            placeholder="逗号或空格分隔"
          />
        </div>
        <div className="admin-form__row">
          <label htmlFor="admin-tag-rule-keywords">包含词（子串）</label>
          <textarea
            id="admin-tag-rule-keywords"
            value={keywords}
            onChange={(e) => setKeywords(e.target.value)}
            placeholder="例如：蜜桃臀, 翘臀"
          />
        </div>
        <div className="admin-form__row">
          <label htmlFor="admin-tag-rule-words">整词</label>
          <textarea
            id="admin-tag-rule-words"
            value={words}
            onChange={(e) => setWords(e.target.value)}
            placeholder="单字和短 ASCII 词建议放在这里"
          />
        </div>
        <div className="admin-form__row">
          <label htmlFor="admin-tag-rule-excludes">排除词</label>
          <textarea
            id="admin-tag-rule-excludes"
            value={excludes}
            onChange={(e) => setExcludes(e.target.value)}
            placeholder="命中这些词的区域不会触发本标签"
          />
        </div>
        {tag.label.toUpperCase() === "AV" && (
          <label className="admin-check admin-tag-rule-av">
            <input
              type="checkbox"
              checked={matchAvCode}
              onChange={(e) => setMatchAvCode(e.target.checked)}
            />
            <span>识别文件名和标题中的番号</span>
          </label>
        )}
        <p className="admin-form__help">
              保存会立即对该标签做增量匹配；如需撤销旧规则产生的自动标签，请运行“重新整理所有标签”。
        </p>
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

function sourceLabel(source: string, tag?: api.AdminTag): string {
  if (tag?.crawlerOwned) return "爬虫脚本";
  if (source === "builtin") return "内置";
  if (source === "user") return "自定义";
  if (source === "generated") return "自动生成";
  return source || "未知";
}
