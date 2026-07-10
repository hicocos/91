import assert from "node:assert/strict";
import test from "node:test";
import { fetchHomeVideos, fetchListing } from "../src/data/videos";

test("home recommendations use a body-free GET request", async (t) => {
  const originalFetch = globalThis.fetch;
  let requestPath = "";
  let requestInit: RequestInit | undefined;
  globalThis.fetch = (async (input, init) => {
    requestPath = String(input);
    requestInit = init;
    return new Response("[]", {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  }) as typeof fetch;
  t.after(() => {
    globalThis.fetch = originalFetch;
  });

  const result = await fetchHomeVideos([" video-1 ", "", "video-2"]);

  assert.deepEqual(result, []);
  assert.equal(requestPath, "/api/home");
  assert.equal(requestInit?.method, undefined);
  assert.equal(requestInit?.body, undefined);
  assert.equal(requestInit?.cache, "no-store");
  assert.equal(new Headers(requestInit?.headers).get("Accept"), "application/json");
});

test("home recommendations retry one transient GET failure", async (t) => {
  const originalFetch = globalThis.fetch;
  let calls = 0;
  globalThis.fetch = (async () => {
    calls += 1;
    if (calls === 1) {
      return new Response("unavailable", { status: 503 });
    }
    return new Response(JSON.stringify([{ id: "video-after-retry" }]), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  }) as typeof fetch;
  t.after(() => {
    globalThis.fetch = originalFetch;
  });

  const result = await fetchHomeVideos();

  assert.equal(calls, 2);
  assert.deepEqual(result.map((item) => item.id), ["video-after-retry"]);
});

test("home recommendations replace recently shown candidates", async (t) => {
  const originalFetch = globalThis.fetch;
  const batches = [
    [{ id: "recent-video" }, { id: "first-new-video" }],
    [{ id: "second-new-video" }, { id: "recent-video" }],
  ];
  let calls = 0;
  const requestInits: Array<RequestInit | undefined> = [];
  globalThis.fetch = (async (_input, init) => {
    requestInits.push(init);
    const body = JSON.stringify(batches[calls++] ?? []);
    return new Response(body, {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  }) as typeof fetch;
  t.after(() => {
    globalThis.fetch = originalFetch;
  });

  const result = await fetchHomeVideos(["recent-video"]);

  assert.equal(calls, 2);
  assert.ok(requestInits.every((init) => init?.method === undefined));
  assert.ok(requestInits.every((init) => init?.body === undefined));
  assert.deepEqual(
    result.map((item) => item.id),
    ["first-new-video", "second-new-video"]
  );
});

test("home recommendations keep the first batch when replacement fails", async (t) => {
  const originalFetch = globalThis.fetch;
  let calls = 0;
  globalThis.fetch = (async () => {
    calls += 1;
    if (calls === 1) {
      return new Response(
        JSON.stringify([{ id: "recent-video" }, { id: "new-video" }]),
        { status: 200, headers: { "Content-Type": "application/json" } }
      );
    }
    return new Response("unavailable", { status: 503 });
  }) as typeof fetch;
  t.after(() => {
    globalThis.fetch = originalFetch;
  });

  const result = await fetchHomeVideos(["recent-video"]);

  assert.equal(calls, 3);
  assert.deepEqual(
    result.map((item) => item.id),
    ["recent-video", "new-video"]
  );
});

test("home recommendation request failures remain observable", async (t) => {
  const originalFetch = globalThis.fetch;
  let calls = 0;
  globalThis.fetch = (async () => {
    calls += 1;
    return new Response("unavailable", { status: 503 });
  }) as typeof fetch;
  t.after(() => {
    globalThis.fetch = originalFetch;
  });

  await assert.rejects(() => fetchHomeVideos(), /HTTP 503/);
  assert.equal(calls, 2);
});

test("listing request failures are not converted to an empty library", async (t) => {
  const originalFetch = globalThis.fetch;
  let calls = 0;
  globalThis.fetch = (async () => {
    calls += 1;
    return new Response("unauthorized", { status: 401 });
  }) as typeof fetch;
  t.after(() => {
    globalThis.fetch = originalFetch;
  });

  await assert.rejects(
    () => fetchListing(1, 96, { sort: "latest", includeTotal: false }),
    /HTTP 401/
  );
  assert.equal(calls, 1);
});
