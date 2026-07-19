import assert from "node:assert/strict";
import test from "node:test";
import {
  HTTPStatusError,
  fetchVideoDetail,
  fetchVideoSubtitles,
} from "../src/data/videos";

test("video detail preserves 404 status instead of returning null", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => new Response("missing", { status: 404 });
  try {
    await assert.rejects(fetchVideoDetail("missing"), (error: unknown) => {
      return error instanceof HTTPStatusError && error.status === 404;
    });
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("video subtitles preserve upstream failures", async () => {
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => new Response("failed", { status: 500 });
  try {
    await assert.rejects(fetchVideoSubtitles("video-1"), (error: unknown) => {
      return error instanceof HTTPStatusError && error.status === 500;
    });
  } finally {
    globalThis.fetch = originalFetch;
  }
});
