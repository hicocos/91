import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const indexHtml = readFileSync(
  new URL("../index.html", import.meta.url),
  "utf8"
);

test("app shell prevents referrer leakage for 302 video playback", () => {
  assert.match(indexHtml, /<meta\s+name="referrer"\s+content="no-referrer"\s*\/?>/);
});

test("theme bootstrap is external so production CSP can forbid inline scripts", () => {
  assert.match(indexHtml, /<script\s+src="\/theme-bootstrap\.js"><\/script>/);
  assert.doesNotMatch(indexHtml, /<script>\s*\(function/);
});
