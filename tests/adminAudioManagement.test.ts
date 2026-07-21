import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path: string) => readFileSync(path, "utf8");

test("admin exposes a dedicated audio management route and navigation item", () => {
  const app = read("src/App.tsx");
  const layout = read("src/admin/AdminLayout.tsx");
  assert.match(app, /path="audios"/);
  assert.match(app, /AudiosPage/);
  assert.match(layout, /to="\/admin\/audios"/);
  assert.match(layout, /音频管理/);
  assert.match(layout, /AudioLines/);
});

test("admin audio page shows stats search metadata and management actions", () => {
  const page = read("src/admin/AudiosPage.tsx");
  assert.match(page, /音频总数/);
  assert.match(page, /listAudios/);
  assert.match(page, /audioStats/);
  assert.match(page, /当前库中没有音频/);
  assert.match(page, /格式/);
  assert.match(page, /删除音频/);
});

test("drive cards expose video and audio inventory plus classified scan progress", () => {
  const components = read("src/admin/drive/DriveComponents.tsx");
  const api = read("src/admin/api.ts");
  assert.match(components, /视频数量/);
  assert.match(components, /音频数量/);
  assert.match(components, /audioScannedCount/);
  assert.match(components, /audioAddedCount/);
  assert.match(api, /audioCount: number/);
  assert.match(api, /videoCount: number/);
});
