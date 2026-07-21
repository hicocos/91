import type { AudioDetail, AudioItem } from "@/types";

async function audioGet<T>(path: string): Promise<T> {
  const response = await fetch(path, {
    credentials: "include",
    cache: "no-store",
    headers: { Accept: "application/json" },
  });
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  return response.json() as Promise<T>;
}

export async function fetchAudios(
  page: number,
  pageSize: number,
  params: { q?: string; tag?: string; sort?: string } = {}
): Promise<{ items: AudioItem[]; total: number }> {
  const query = new URLSearchParams({ page: String(page), size: String(pageSize) });
  if (params.q) query.set("q", params.q);
  if (params.tag) query.set("tag", params.tag);
  if (params.sort) query.set("sort", params.sort);
  const result = await audioGet<{ items: AudioItem[]; total: number }>(`/api/audios?${query}`);
  if (!result || !Array.isArray(result.items) || typeof result.total !== "number") {
    throw new Error("Invalid /api/audios response");
  }
  return result;
}

export function fetchAudioDetail(id: string): Promise<AudioDetail> {
  return audioGet<AudioDetail>(`/api/audio/${encodeURIComponent(id)}`);
}

export async function recordAudioView(id: string): Promise<{ views: number }> {
  const response = await fetch(`/api/audio/${encodeURIComponent(id)}/view`, {
    method: "POST",
    credentials: "include",
  });
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
  return response.json() as Promise<{ views: number }>;
}