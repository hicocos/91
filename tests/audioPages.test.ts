import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (path: string) => readFileSync(path, "utf8");

test("audio navigation and routes are separate from video routes", () => {
  const nav = read("src/components/MainNav.tsx");
  const app = read("src/App.tsx");
  assert.match(nav, /AudioLines/);
  assert.match(nav, /to: "\/audio", label: "音频"/);
  assert.match(app, /path="\/audio"/);
  assert.match(app, /path="\/audio\/:id"/);
  assert.match(app, /AudioLibraryPage/);
  assert.match(app, /AudioDetailPage/);
});

test("audio client validates dedicated API responses", () => {
  const source = read("src/data/audios.ts");
  assert.match(source, /\/api\/audios\?/);
  assert.match(source, /\/api\/audio\/\$\{encodeURIComponent\(id\)\}/);
  assert.match(source, /\/api\/audio\/\$\{encodeURIComponent\(id\)\}\/view/);
  assert.match(source, /Array\.isArray\(result\.items\)/);
});

test("audio pages use a semantic player and audio-specific states", () => {
  const library = read("src/pages/AudioLibraryPage.tsx");
  const detail = read("src/pages/AudioDetailPage.tsx");
  const player = read("src/components/AudioPlayer.tsx");
  assert.match(library, /当前库中没有音频/);
  assert.match(library, /音频列表加载失败/);
  assert.match(detail, /当前浏览器可能不支持此音频格式/);
  assert.match(player, /<audio/);
  assert.match(player, /preload="metadata"/);
  assert.match(player, /onPlay/);
});

test("audio layout has explicit responsive constraints", () => {
  const css = read("src/styles/audio.css");
  const main = read("src/main.tsx");
  assert.match(css, /@media \(max-width: 768px\)/);
  assert.match(css, /\.audio-track__format/);
  assert.match(css, /minmax\(0, 1fr\)/);
  assert.match(main, /styles\/audio\.css/);
});