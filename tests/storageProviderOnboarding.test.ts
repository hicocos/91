import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const api = readFileSync(new URL("../src/admin/api.ts", import.meta.url), "utf8");
const constants = readFileSync(new URL("../src/admin/drive/constants.ts", import.meta.url), "utf8");
const form = readFileSync(new URL("../src/admin/drive/DriveForm.tsx", import.meta.url), "utf8");
const page = readFileSync(new URL("../src/admin/DrivesPage.tsx", import.meta.url), "utf8");

test("storage onboarding exposes manifest, S3, OAuth and backend-bound test-before-save UX", () => {
 assert.match(api, /listStorageProviders/);
 assert.match(api, /saveStorageAccount/);
 assert.match(api, /getStorageAccount/);
 assert.match(api, /StorageProviderManifest/);
 assert.match(constants, /"s3"/);
 assert.match(form, /授权连接/);
 assert.match(page, /测试并添加|测试并保存/);
 assert.match(page, /saveStorageAccount/);
 assert.doesNotMatch(page, /await api\.probeStorageAccount[\s\S]*await api\.upsertDrive/);
 assert.doesNotMatch(api, /writable/);
 assert.doesNotMatch(page, /writable\s*:/);
 assert.match(page, /listStorageProviders/);
 assert.match(form, /providerManifest && !providerManifest\.legacy/);
 assert.match(form, /partitionProviderFields/);
 assert.match(page, /"configured" in result/);
 assert.match(form, /已配置，留空则保持不变/);
});
