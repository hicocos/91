import type { VideoDetail, VideoItem, VideoSubtitle } from "@/types";

// 真实后端接口调用。未配置网盘时，各接口返回空数据。
export async function fetchHomeVideos(excludeIds?: string[]): Promise<VideoItem[]> {
  const recentIds = new Set(
    (excludeIds ?? [])
      .map((id) => id.trim())
      .filter((id) => id.length > 0)
  );
  const firstBatch = await apiGet<VideoItem[]>("/api/home");
  if (!Array.isArray(firstBatch)) {
    throw new Error("Invalid /api/home response");
  }
  if (recentIds.size === 0 || firstBatch.every((item) => !recentIds.has(item.id))) {
    return firstBatch;
  }

  // Some client networks fail to deliver segmented request bodies reliably, so
  // keep this request body-free. A second random batch replaces recent items.
  let candidates = firstBatch;
  try {
    const secondBatch = await apiGet<VideoItem[]>("/api/home");
    candidates = [...firstBatch, ...secondBatch];
  } catch {
    return firstBatch;
  }
  const selected: VideoItem[] = [];
  const selectedIds = new Set<string>();
  for (const includeRecent of [false, true]) {
    for (const item of candidates) {
      if (!item?.id || selectedIds.has(item.id)) continue;
      if (recentIds.has(item.id) !== includeRecent) continue;
      selected.push(item);
      selectedIds.add(item.id);
      if (selected.length >= firstBatch.length) return selected;
    }
  }
  return selected;
}

export async function fetchListing(
  page: number,
  pageSize: number,
  params?: { q?: string; tag?: string; sort?: string; includeTotal?: boolean }
): Promise<{ items: VideoItem[]; total: number }> {
  const qs = new URLSearchParams({
    page: String(page),
    size: String(pageSize),
  });
  if (params?.q) qs.set("q", params.q);
  if (params?.tag) qs.set("tag", params.tag);
  if (params?.sort) qs.set("sort", params.sort);
  if (params?.includeTotal === false) qs.set("count", "false");
  const result = await apiGet<{ items: VideoItem[]; total: number }>(
    `/api/list?${qs.toString()}`
  );
  if (
    !result ||
    !Array.isArray(result.items) ||
    typeof result.total !== "number"
  ) {
    throw new Error("Invalid /api/list response");
  }
  return result;
}

export function fetchVideoDetail(id: string): Promise<VideoDetail | null> {
  return apiGet<VideoDetail>(`/api/video/${encodeURIComponent(id)}`).catch(
    () => null
  );
}

export function fetchVideoSubtitles(id: string): Promise<VideoSubtitle[]> {
  return apiGet<VideoSubtitle[]>(
    `/api/video/${encodeURIComponent(id)}/subtitles`
  ).catch(() => []);
}

export function updateVideoTags(
  id: string,
  tags: string[]
): Promise<VideoItem> {
  return apiJSON<VideoItem>(`/api/video/${encodeURIComponent(id)}/tags`, {
    method: "PUT",
    body: JSON.stringify({ tags }),
  });
}

export function hideVideo(id: string): Promise<{ ok: boolean }> {
  return apiJSON<{ ok: boolean }>(
    `/api/video/${encodeURIComponent(id)}/hide`,
    { method: "POST" }
  );
}

export function deleteVideo(
  id: string,
  options: { deleteSource?: boolean } = {}
): Promise<{ ok: boolean; deletedSource: boolean }> {
  return apiJSON<{ ok: boolean; deletedSource: boolean }>(
    `/admin/api/videos/${encodeURIComponent(id)}`,
    {
      method: "DELETE",
      body: JSON.stringify({ deleteSource: !!options.deleteSource }),
    }
  );
}

export function recordView(id: string): Promise<{ views: number }> {
  return apiJSON<{ views: number }>(
    `/api/video/${encodeURIComponent(id)}/view`,
    { method: "POST" }
  );
}

export type UploadVideoInput = {
  file: File;
  title: string;
  tags: string[];
};

export function uploadVideo(input: UploadVideoInput): Promise<VideoItem> {
  const body = new FormData();
  body.append("file", input.file);
  if (input.title.trim()) {
    body.append("title", input.title.trim());
  }
  for (const tag of input.tags) {
    body.append("tags", tag);
  }
  return apiForm<VideoItem>("/api/upload", body);
}

export type TagItem = { id: string; label: string; count?: number };

const TAG_CACHE_TTL_MS = 30_000;
let cachedTags: TagItem[] | null = null;
let cachedTagsAt = 0;
let pendingTags: Promise<TagItem[]> | null = null;

export function fetchTags(): Promise<TagItem[]> {
  const now = Date.now();
  if (cachedTags && now - cachedTagsAt < TAG_CACHE_TTL_MS) {
    return Promise.resolve(cachedTags);
  }
  if (pendingTags) return pendingTags;
  pendingTags = apiGet<TagItem[]>("/api/tags")
    .then((tags) => {
      cachedTags = tags;
      cachedTagsAt = Date.now();
      return tags;
    })
    .catch(() => cachedTags ?? [])
    .finally(() => {
      pendingTags = null;
    });
  return pendingTags;
}

/** 短视频模式单条记录。比 VideoItem 多 videoSrc / poster。 */
export type ShortsItem = VideoItem & {
  videoSrc: string;
  poster: string;
};

export type ShortsFeedItem = ShortsItem & {
  /** Resume position immediately after this item in the server-side feed. */
  feedCursor: number;
};

/** 短视频"取下一批"接口的响应。 */
export type ShortsNextResponse = {
  items: ShortsFeedItem[];
  total: number;
  feedToken: string;
  nextCursor: number;
  /** true 表示当前服务端随机轮次已经读到末尾。 */
  roundComplete: boolean;
};

export class ShortsFeedExpiredError extends Error {
  constructor() {
    super("Shorts feed expired");
    this.name = "ShortsFeedExpiredError";
  }
}

/**
 * 拉取服务端随机 feed 的下一批候选。请求只携带固定大小的令牌和游标，
 * 不会再随已看视频数量增长，也没有请求体。
 */
export async function fetchShortsNext(
  feedToken: string,
  cursor: number,
  count: number
): Promise<ShortsNextResponse> {
  const params = new URLSearchParams({
    cursor: String(cursor),
    count: String(count),
  });
  if (feedToken) params.set("feedToken", feedToken);

  let result: ShortsNextResponse;
  try {
    result = await apiGet<ShortsNextResponse>(
      `/api/shorts/next?${params.toString()}`
    );
  } catch (error) {
    if (error instanceof HTTPStatusError && error.status === 410) {
      throw new ShortsFeedExpiredError();
    }
    throw error;
  }

  if (
    !result ||
    !Array.isArray(result.items) ||
    !Number.isInteger(result.total) ||
    result.total < 0 ||
    typeof result.feedToken !== "string" ||
    (result.total > 0 && result.feedToken.length === 0) ||
    !Number.isInteger(result.nextCursor) ||
    result.nextCursor < 0 ||
    typeof result.roundComplete !== "boolean" ||
    result.items.some(
      (item) =>
        !Number.isInteger(item.feedCursor) ||
        item.feedCursor < 1 ||
        item.feedCursor > result.nextCursor
    )
  ) {
    throw new Error("Invalid /api/shorts/next response");
  }
  return result;
}

const API_GET_MAX_ATTEMPTS = 2;
const API_GET_RETRY_DELAY_MS = 200;
const API_GET_TIMEOUT_MS = 10_000;

class HTTPStatusError extends Error {
  constructor(readonly status: number) {
    super(`HTTP ${status}`);
    this.name = "HTTPStatusError";
  }
}

function isRetryableGetError(error: unknown): boolean {
  if (!(error instanceof HTTPStatusError)) return true;
  return error.status === 408 || error.status === 425 || error.status === 429 || error.status >= 500;
}

function wait(ms: number): Promise<void> {
  return new Promise((resolve) => globalThis.setTimeout(resolve, ms));
}

async function apiGet<T>(path: string): Promise<T> {
  let lastError: unknown;

  for (let attempt = 1; attempt <= API_GET_MAX_ATTEMPTS; attempt += 1) {
    const controller = new AbortController();
    const timeoutID = globalThis.setTimeout(
      () => controller.abort(),
      API_GET_TIMEOUT_MS
    );
    try {
      const res = await fetch(path, {
        credentials: "include",
        cache: "no-store",
        headers: { Accept: "application/json" },
        signal: controller.signal,
      });
      if (!res.ok) throw new HTTPStatusError(res.status);
      return (await res.json()) as T;
    } catch (error) {
      lastError = error;
      if (attempt >= API_GET_MAX_ATTEMPTS || !isRetryableGetError(error)) {
        throw error;
      }
    } finally {
      globalThis.clearTimeout(timeoutID);
    }

    await wait(API_GET_RETRY_DELAY_MS);
  }

  throw lastError instanceof Error ? lastError : new Error("API request failed");
}

async function apiJSON<T>(path: string, init: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

async function apiForm<T>(path: string, body: FormData): Promise<T> {
  const res = await fetch(path, {
    method: "POST",
    credentials: "include",
    body,
  });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}
