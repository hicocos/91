import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { me, UnauthorizedError } from "../src/admin/api.ts";
import {
  subscribeUnauthorized,
} from "../src/admin/authEvents.ts";
import { safeReturnPath } from "../src/admin/safeReturnPath.ts";

test("safe return path accepts only local absolute paths", () => {
  assert.equal(safeReturnPath("/admin?x=1", "/"), "/admin?x=1");
  assert.equal(safeReturnPath("//evil.example", "/admin"), "/admin");
  assert.equal(safeReturnPath("\\\\evil", "/admin"), "/admin");
  assert.equal(safeReturnPath("https://evil.example", "/admin"), "/admin");
});

test("unauthorized event fires exactly once for a 401 response", async () => {
  const originalFetch = globalThis.fetch;
  let unauthorizedEvents = 0;
  const unsubscribe = subscribeUnauthorized(() => {
    unauthorizedEvents += 1;
  });
  globalThis.fetch = async () => new Response("unauthorized", { status: 401 });

  try {
    await assert.rejects(me(), UnauthorizedError);
    assert.equal(unauthorizedEvents, 1);
  } finally {
    unsubscribe();
    globalThis.fetch = originalFetch;
  }
});

test("auth provider exposes unavailable status and subscribes to session invalidation", () => {
  const source = readFileSync(
    new URL("../src/admin/AuthContext.tsx", import.meta.url),
    "utf8"
  );

  assert.match(source, /"unavailable"/);
  assert.match(source, /subscribeUnauthorized/);
  assert.match(source, /setStatus\("unavailable"\)/);
});

test("login routes sanitize return paths and expose a recoverable connection error", () => {
  const requireAuthSource = readFileSync(
    new URL("../src/admin/RequireAuth.tsx", import.meta.url),
    "utf8"
  );
  const loginPageSource = readFileSync(
    new URL("../src/admin/LoginPage.tsx", import.meta.url),
    "utf8"
  );

  assert.match(requireAuthSource, /safeReturnPath/);
  assert.match(requireAuthSource, /status === "unavailable"/);
  assert.match(requireAuthSource, /onClick=\{refresh\}/);
  assert.match(loginPageSource, /safeReturnPath/);
  assert.match(loginPageSource, /status === "unavailable"/);
  assert.match(loginPageSource, /onClick=\{refresh\}/);
});
