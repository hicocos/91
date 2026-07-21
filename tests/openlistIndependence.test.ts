import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import test from "node:test";

const rootFile = (path: string) => new URL(`../${path}`, import.meta.url);
const read = (path: string) => readFileSync(rootFile(path), "utf8");

const productionAndDependencyFiles = [
  "backend/internal/drives/onedrive/driver.go",
  "backend/cmd/server/drives.go",
  "backend/internal/api/admin_storage.go",
  "backend/internal/drives/wopan/driver.go",
  "backend/internal/drives/wopan/driver_test.go",
  "backend/go.mod",
  "backend/go.sum",
  "backend/vendor/modules.txt",
];

test("runtime and build sources have no OpenList private API or module dependency", () => {
  const source = productionAndDependencyFiles.map((path) => `${path}\n${read(path)}`).join("\n");
  assert.doesNotMatch(source, /api\.oplist\.org|renewapi|refresh_ui|driver_txt/);
  assert.doesNotMatch(source, /github\.com\/OpenListTeam\/wopan-sdk-go/);
  assert.doesNotMatch(source, /RenewAPIURL|renewAPIURL|defaultRenewAPIURL/);
  assert.equal(existsSync(rootFile("backend/vendor/github.com/OpenListTeam/wopan-sdk-go")), false);
});

test("standard WebDAV remains supported without vendor-specific setup copy", () => {
  const registry = read("backend/internal/storageproviders/registry.go");
  const driver = read("backend/internal/drives/webdav/driver.go");
  const frontend = read("src/admin/drive/constants.ts");
  const docs = [read("README.md"), read("backend/README.md"), read("backend/config.example.yaml")].join("\n");

  assert.match(registry, /Kind: "webdav"/);
  assert.match(driver, /PROPFIND/);
  assert.match(frontend, /key: "base_url"/);
  assert.doesNotMatch(frontend, /openlist\.example|OpenList OneDrive/);
  assert.doesNotMatch(docs, /api\.oplist\.org|OpenList 代刷|OpenList 默认应用方式/);
});
