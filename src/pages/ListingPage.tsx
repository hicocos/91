import { useEffect, useRef, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { AppShell } from "@/components/AppShell";
import { PromoStrip } from "@/components/PromoStrip";
import { SearchPanel } from "@/components/SearchPanel";
import { TagCloud } from "@/components/TagCloud";
import { SortToolbar, type ViewMode } from "@/components/SortToolbar";
import { VideoGrid } from "@/components/VideoGrid";
import { Pagination } from "@/components/Pagination";
import { AdminEmptyVisual } from "@/admin/AdminEmptyVisual";
import { fetchListing } from "@/data/videos";
import type { SortKey, VideoItem } from "@/types";

const PAGE_SIZE_DEFAULT = 24;
const PAGE_SIZE_TAG = 12;

type ListingContentProps = {
  keyword: string;
  tag: string;
};

export default function ListingPage() {
  const [params] = useSearchParams();
  const keyword = params.get("q") ?? "";
  const tag = params.get("tag") ?? "";

  return <ListingContent key={`${keyword}\n${tag}`} keyword={keyword} tag={tag} />;
}

function ListingContent({ keyword, tag }: ListingContentProps) {
  const hasLoadedListingRef = useRef(false);

  const [sort, setSort] = useState<SortKey>("hot");
  const [view, setView] = useState<ViewMode>("grid");
  const [page, setPage] = useState(1);
  const [initialLoading, setInitialLoading] = useState(true);
  const [listingError, setListingError] = useState(false);
  const [items, setItems] = useState<VideoItem[]>([]);
  const [total, setTotal] = useState(0);
  const hasActiveFilter = keyword.trim().length > 0 || tag.trim().length > 0;

  useEffect(() => {
    document.title = keyword
      ? `搜索 "${keyword}"`
      : tag
      ? `标签 ${tag}`
      : "视频列表";

    let active = true;
    const isInitialLoad = !hasLoadedListingRef.current;
    if (isInitialLoad) {
      setInitialLoading(true);
    }
    setListingError(false);
    fetchListing(page, tag ? PAGE_SIZE_TAG : PAGE_SIZE_DEFAULT, { q: keyword, tag, sort })
      .then((r) => {
        if (!active) return;
        setItems(r.items ?? []);
        setTotal(r.total ?? 0);
        hasLoadedListingRef.current = true;
      })
      .catch(() => {
        if (active) setListingError(true);
      })
      .finally(() => {
        if (active) setInitialLoading(false);
      });
    return () => {
      active = false;
    };
  }, [keyword, tag, sort, page]);

  return (
    <AppShell>
      <div className="container page-section listing-discovery-section">
        <PromoStrip />
        <SearchPanel />
        <TagCloud />
      </div>

      <div className="container page-section listing-primary-section">
        <SortToolbar
          sort={sort}
          view={view}
          onSortChange={(nextSort) => {
            setSort(nextSort);
            setPage(1);
            window.scrollTo({ top: 0, behavior: "smooth" });
          }}
          onViewChange={(nextView) => {
            setView(nextView);
          }}
        />
        {initialLoading ? (
          <VideoGrid videos={items} loading compact={view === "compact"} skeletonCount={12} />
        ) : listingError && items.length === 0 ? (
          <AdminEmptyVisual
            variant="no-results"
            text="视频列表加载失败，请刷新重试"
            className="admin-empty-state admin-empty-state--plain listing-empty-state"
          />
        ) : items.length === 0 ? (
          <AdminEmptyVisual
            variant={hasActiveFilter ? "no-results" : "empty"}
            text={hasActiveFilter ? "未查询到" : "当前库中没有视频"}
            className="admin-empty-state admin-empty-state--plain listing-empty-state"
          />
        ) : (
          <VideoGrid videos={items} compact={view === "compact"} skeletonCount={12} />
        )}
        <Pagination
          page={page}
          pageSize={tag ? PAGE_SIZE_TAG : PAGE_SIZE_DEFAULT}
          total={total}
          onChange={(p) => {
            setPage(p);
            window.scrollTo({ top: 0, behavior: "smooth" });
          }}
        />
      </div>
    </AppShell>
  );
}
