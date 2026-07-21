import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { AppShell } from "@/components/AppShell";
import { AudioTrackList } from "@/components/AudioTrackList";
import { SearchPanel } from "@/components/SearchPanel";
import { Pagination } from "@/components/Pagination";
import { fetchAudios } from "@/data/audios";
import type { AudioItem, SortKey } from "@/types";

const PAGE_SIZE = 30;

export default function AudioLibraryPage() {
  const [params, setParams] = useSearchParams();
  const keyword = params.get("q")?.trim() ?? "";
  const [sort, setSort] = useState<SortKey>("latest");
  const [page, setPage] = useState(1);
  const [items, setItems] = useState<AudioItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    document.title = keyword ? `搜索音频 “${keyword}”` : "音频";
    let active = true;
    setLoading(true);
    setFailed(false);
    fetchAudios(page, PAGE_SIZE, { q: keyword, sort })
      .then((result) => {
        if (!active) return;
        setItems(result.items);
        setTotal(result.total);
      })
      .catch(() => {
        if (active) setFailed(true);
      })
      .finally(() => {
        if (active) setLoading(false);
      });
    return () => { active = false; };
  }, [keyword, page, sort]);

  return (
    <AppShell>
      <div className="container audio-library">
        <header className="audio-library__hero">
          <span className="audio-library__eyebrow">AUDIO LIBRARY</span>
          <h1>音频</h1>
          <p>网盘中的音乐、录音与无损音频，与视频内容独立展示。</p>
        </header>
        <SearchPanel
          value={keyword}
          onSearch={(value) => {
            const next = new URLSearchParams(params);
            const q = value.trim();
            if (q) next.set("q", q); else next.delete("q");
            setParams(next, { replace: true });
            setPage(1);
          }}
        />
        <div className="audio-library__toolbar">
          <strong>{total} 条音频</strong>
          <label>
            排序
            <select value={sort} onChange={(event) => { setSort(event.target.value as SortKey); setPage(1); }}>
              <option value="latest">最新收录</option>
              <option value="hot">最多点赞</option>
              <option value="recent">最近播放</option>
            </select>
          </label>
        </div>
        {loading ? (
          <div className="audio-library__state" aria-live="polite">音频列表加载中...</div>
        ) : failed ? (
          <div className="audio-library__state is-error">音频列表加载失败，请刷新重试</div>
        ) : items.length === 0 ? (
          <div className="audio-library__state">{keyword ? "未查询到音频" : "当前库中没有音频"}</div>
        ) : (
          <AudioTrackList items={items} />
        )}
        <Pagination page={page} pageSize={PAGE_SIZE} total={total} onChange={setPage} />
      </div>
    </AppShell>
  );
}