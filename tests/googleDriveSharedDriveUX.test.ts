import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const form = readFileSync(new URL("../src/admin/drive/DriveForm.tsx", import.meta.url), "utf8");
const constants = readFileSync(new URL("../src/admin/drive/constants.ts", import.meta.url), "utf8");
const docs = readFileSync(new URL("../src/admin/MountDocsPage.tsx", import.meta.url), "utf8");

test("Google Drive form explains personal shared drive and folder scopes in Chinese", () => {
  assert.match(form, /Google Drive 挂载范围说明/);
  assert.match(form, /个人盘：团队盘 ID 留空/);
  assert.match(form, /整个团队盘：填写“共享云端硬盘（团队盘）ID”/);
  assert.match(form, /团队盘中的子文件夹/);
  assert.match(constants, /扫描起点文件夹 ID（可选）/);
  assert.match(constants, /团队盘 ID 填上方字段，这里留空/);
});

test("mount docs contain complete Google Drive personal and shared drive examples", () => {
  assert.match(docs, /共享云端硬盘（团队盘）ID/);
  assert.match(docs, /扫描起点文件夹 ID（可选）/);
  assert.match(docs, /扫描整个“我的云端硬盘”/);
  assert.match(docs, /扫描整个团队盘/);
  assert.match(docs, /只扫描团队盘中的某个文件夹/);
  assert.match(docs, /folders\/0Axxxxxxxxxxxxxxxxx/);
  assert.match(docs, /授权账号必须是该团队盘的成员/);
  assert.match(docs, /扫描结果为 0/);
});
