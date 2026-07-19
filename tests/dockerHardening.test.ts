import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const dockerfile = readFileSync(new URL("../Dockerfile", import.meta.url), "utf8");
const entrypoint = readFileSync(
  new URL("../docker-entrypoint.sh", import.meta.url),
  "utf8"
);
const compose = readFileSync(
  new URL("../docker-compose.yml", import.meta.url),
  "utf8"
);

test("runtime image uses the fixed unprivileged uid and gid", () => {
  assert.match(dockerfile, /groupadd[^\n]*--gid[= ]9191/);
  assert.match(dockerfile, /useradd[^\n]*--uid[= ]9191[^\n]*--gid[= ][^\s]+/);
  assert.match(dockerfile, /chown[^\n]*\/opt\/video-site-91/);
  assert.match(dockerfile, /^USER (?:9191(?::9191)?|video-site-91)$/m);
  assert.doesNotMatch(entrypoint, /(?:^|\s)(?:chown|su|gosu|sudo)(?:\s|$)/m);
});

test("compose keeps the local hicocos image and repository overrides", () => {
  assert.match(compose, /^\s*image:\s*hicocos-91:local\s*$/m);
  assert.match(compose, /^\s*VIDEO_GITHUB_REPO:\s*hicocos\/91\s*$/m);
  assert.match(compose, /^\s*VERSION:\s*\$\{VERSION:-dev\}\s*$/m);
});

test("compose binds only to loopback and mounts the persistent data target", () => {
  assert.match(compose, /127\.0\.0\.1:9191:9191/);
  assert.match(compose, /\.\/data:\/opt\/video-site-91\/data/);
  assert.doesNotMatch(compose, /:\/www\/91\/data/);
  assert.match(compose, /healthcheck:[\s\S]*\/healthz/);
});

test("compose constrains filesystem privileges and process privileges", () => {
  assert.match(compose, /^\s*read_only:\s*true\s*$/m);
  assert.match(compose, /^\s*tmpfs:\s*$/m);
  assert.match(compose, /^\s*-\s*\/tmp(?::|\s*$)/m);
  assert.match(compose, /^\s*cap_drop:\s*$/m);
  assert.match(compose, /^\s*-\s*ALL\s*$/m);
  assert.match(compose, /no-new-privileges:true/);
  assert.match(compose, /^\s*init:\s*true\s*$/m);
});

test("compose defines resource, pid, and rotating log limits", () => {
  assert.match(compose, /^\s*mem_limit:\s*\S+\s*$/m);
  assert.match(compose, /^\s*cpus:\s*["']?[0-9.]+["']?\s*$/m);
  assert.match(compose, /^\s*pids_limit:\s*[0-9]+\s*$/m);
  assert.match(compose, /logging:[\s\S]*driver:\s*["']?json-file["']?/);
  assert.match(compose, /max-size:\s*["']?\S+["']?/);
  assert.match(compose, /max-file:\s*["']?[0-9]+["']?/);
});
