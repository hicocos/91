import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { Edit, Search, Trash2 } from "lucide-react";
import * as api from "./api";
import { formatBytes } from "./storageFormat";
import { useToast } from "./ToastContext";
import { Modal } from "./Modal";
import { ConfirmModal } from "./ConfirmModal";
import { AdminEmptyVisual } from "./AdminEmptyVisual";

const PAGE_SIZE = 30;

export function AudiosPage() {
  const [items, setItems] = useState<api.AdminVideo[]>([]);
  const [drives, setDrives] = useState<api.AdminDrive[]>([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [keyword, setKeyword] = useState("");
  const [query, setQuery] = useState("");
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState<api.AdminVideo | null>(null);
  const [deleting, setDeleting] = useState<api.AdminVideo | null>(null);
  const [title, setTitle] = useState("");
  const [author, setAuthor] = useState("");
  const { show } = useToast();

  async function refresh() {
    setLoading(true);
    try {
      const [list, stats, driveList] = await Promise.all([
        api.listAudios({ page, size: PAGE_SIZE, keyword: query }),
        api.audioStats(),
        api.listDrives(),
      ]);
      setItems(list.items ?? []);
      setTotal(stats.current ?? list.total ?? 0);
      setDrives(driveList ?? []);
    } catch (error) {
      show(error instanceof Error ? error.message : "音频列表加载失败", "error");
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => { refresh(); }, [page, query]);
  const driveNames = new Map(drives.map((drive) => [drive.id, drive.name || drive.id]));
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  function openEdit(item: api.AdminVideo) {
    setEditing(item);
    setTitle(item.title);
    setAuthor(item.author || "");
  }

  async function saveEdit() {
    if (!editing || !title.trim()) return;
    try {
      await api.updateVideo(editing.id, { title: title.trim(), author: author.trim() });
      setEditing(null);
      show("音频信息已更新", "success");
      refresh();
    } catch (error) {
      show(error instanceof Error ? error.message : "保存失败", "error");
    }
  }

  async function deleteAudio() {
    if (!deleting) return;
    try {
      await api.deleteVideo(deleting.id);
      setDeleting(null);
      show("已删除音频记录", "success");
      refresh();
    } catch (error) {
      show(error instanceof Error ? error.message : "删除失败", "error");
    }
  }

  return (
    <section className="admin-page admin-audios-page">
      <header className="admin-page__header admin-audio-header">
        <div><span className="admin-text-faint">音频总数</span><strong className="admin-audio-total">{total}</strong></div>
        <form onSubmit={(event) => { event.preventDefault(); setPage(1); setQuery(keyword.trim()); }} className="admin-audio-search">
          <Search size={15} /><input value={keyword} onChange={(event) => setKeyword(event.target.value)} placeholder="搜索标题、作者或文件名" /><button className="admin-btn" type="submit">搜索</button>
        </form>
      </header>

      {loading ? <div className="admin-loading-screen">音频加载中...</div> : items.length === 0 ? (
        <AdminEmptyVisual variant={query ? "no-results" : "empty"} text={query ? "未查询到音频" : "当前库中没有音频"} />
      ) : (
        <table className="admin-table admin-audio-table">
          <thead><tr><th>标题</th><th>作者</th><th>格式</th><th>时长</th><th>大小</th><th>来源</th><th>操作</th></tr></thead>
          <tbody>{items.map((item) => (
            <tr key={item.id}>
              <td data-label="标题"><Link to={`/audio/${encodeURIComponent(item.id)}`}><strong>{item.title}</strong><small className="admin-audio-file">{item.fileId}</small></Link></td>
              <td data-label="作者">{item.author || "—"}</td><td data-label="格式"><span className="admin-status">{item.ext?.toUpperCase() || "AUDIO"}</span></td>
              <td data-label="时长">{formatDuration(item.durationSeconds)}</td><td data-label="大小">{formatBytes(item.size)}</td><td data-label="来源">{driveNames.get(item.driveId) ?? item.driveId}</td>
              <td className="is-actions"><button className="admin-btn" title="编辑音频" onClick={() => openEdit(item)}><Edit size={13} /></button>{" "}<button className="admin-btn is-danger" title="删除音频" onClick={() => setDeleting(item)}><Trash2 size={13} /></button></td>
            </tr>
          ))}</tbody>
        </table>
      )}
      {pages > 1 && <div className="admin-audio-pagination"><button className="admin-btn" disabled={page <= 1} onClick={() => setPage((value) => value - 1)}>上一页</button><span>{page} / {pages}</span><button className="admin-btn" disabled={page >= pages} onClick={() => setPage((value) => value + 1)}>下一页</button></div>}

      <Modal open={!!editing} title="编辑音频" onClose={() => setEditing(null)} footer={<><button className="admin-btn" onClick={() => setEditing(null)}>取消</button><button className="admin-btn is-primary" onClick={saveEdit}>保存</button></>}>
        <div className="admin-form"><label>标题<input value={title} onChange={(event) => setTitle(event.target.value)} /></label><label>作者<input value={author} onChange={(event) => setAuthor(event.target.value)} /></label></div>
      </Modal>
      <ConfirmModal open={!!deleting} title="删除音频" message={`确定删除“${deleting?.title ?? ""}”的媒体库记录吗？源文件不会删除。`} confirmText="删除" danger onConfirm={deleteAudio} onCancel={() => setDeleting(null)} />
    </section>
  );
}

function formatDuration(seconds: number) {
  if (!seconds) return "--:--";
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const rest = seconds % 60;
  return hours > 0 ? `${hours}:${String(minutes).padStart(2, "0")}:${String(rest).padStart(2, "0")}` : `${minutes}:${String(rest).padStart(2, "0")}`;
}
