import assert from "node:assert/strict";
import test from "node:test";
import {
  applyS3Preset,
  defaultProviderFormOptions,
  partitionProviderFields,
  rootFieldPlacement,
  validateProviderForm,
  type ProviderFieldLike,
} from "../src/admin/drive/providerFormRules";

const fields = (...keys: string[]): ProviderFieldLike[] =>
  keys.map((key) => ({ key, label: key, required: false }));

test("OAuth providers keep manual and enterprise fields out of the default form", () => {
  const oneDrive = partitionProviderFields(
    "onedrive",
    fields("client_id", "client_secret", "tenant", "refresh_token", "site_id", "drive_id"),
    defaultProviderFormOptions("onedrive", {}, false),
  );
  assert.deepEqual(oneDrive.primary.map((field) => field.key), ["client_id"]);
  assert.deepEqual(oneDrive.advanced.map((field) => field.key), ["client_secret", "tenant", "refresh_token"]);

  const google = partitionProviderFields(
    "googledrive",
    fields("client_id", "client_secret", "refresh_token", "shared_drive_id"),
    defaultProviderFormOptions("googledrive", {}, false),
  );
  assert.deepEqual(google.primary.map((field) => field.key), ["client_id", "client_secret"]);
  assert.deepEqual(google.advanced.map((field) => field.key), ["refresh_token", "shared_drive_id"]);
});

test("SharePoint mode reveals site and drive fields without affecting personal OneDrive", () => {
  const allFields = fields("client_id", "client_secret", "tenant", "refresh_token", "site_id", "drive_id");
  const personal = defaultProviderFormOptions("onedrive", {}, false);
  assert.equal(personal.oneDriveTarget, "personal");
  assert.deepEqual(partitionProviderFields("onedrive", allFields, personal).advanced.map((field) => field.key), [
    "client_secret",
    "tenant",
    "refresh_token",
  ]);

  const sharePoint = { ...personal, oneDriveTarget: "sharepoint" as const };
  assert.deepEqual(partitionProviderFields("onedrive", allFields, sharePoint).advanced.map((field) => field.key), [
    "client_secret",
    "tenant",
    "refresh_token",
    "site_id",
    "drive_id",
  ]);
});

test("WebDAV supports anonymous access and only shows credentials for basic authentication", () => {
  const allFields = fields("base_url", "username", "password");
  const basic = defaultProviderFormOptions("webdav", {}, false);
  assert.equal(basic.webdavAuth, "basic");
  assert.deepEqual(partitionProviderFields("webdav", allFields, basic).primary.map((field) => field.key), [
    "base_url",
    "username",
    "password",
  ]);

  const anonymous = { ...basic, webdavAuth: "anonymous" as const };
  assert.deepEqual(partitionProviderFields("webdav", allFields, anonymous).primary.map((field) => field.key), [
    "base_url",
  ]);
  assert.equal(validateProviderForm("webdav", "/", { base_url: "https://dav.example.com" }, anonymous), "");
  assert.equal(
    validateProviderForm("webdav", "/", { base_url: "https://dav.example.com", username: "user" }, basic),
    "请填写WebDAV密码",
  );
});

test("S3 presets fill protocol defaults and use the shared root field as the only scan prefix", () => {
  assert.deepEqual(applyS3Preset("aws", { endpoint: "https://old.example", region: "auto", force_path_style: "true" }), {
    endpoint: "",
    region: "us-east-1",
    force_path_style: "false",
  });
  assert.deepEqual(applyS3Preset("r2", { endpoint: "https://account.r2.cloudflarestorage.com" }), {
    endpoint: "https://account.r2.cloudflarestorage.com",
    region: "auto",
    force_path_style: "true",
  });
  assert.deepEqual(applyS3Preset("minio", { endpoint: "https://minio.example.com" }), {
    endpoint: "https://minio.example.com",
    region: "us-east-1",
    force_path_style: "true",
  });
  assert.equal(rootFieldPlacement("s3"), "primary");
  assert.equal(rootFieldPlacement("onedrive"), "advanced");

  const options = defaultProviderFormOptions("s3", {}, false);
  assert.equal(
    validateProviderForm("s3", "", {
      bucket: "media",
      access_key_id: "key",
      secret_access_key: "secret",
      region: "us-east-1",
    }, options),
    "请填写扫描目录前缀",
  );
});

test("OAuth save validation asks for authorization instead of exposing refresh tokens as routine input", () => {
  const oneDrive = defaultProviderFormOptions("onedrive", {}, false);
  assert.equal(
    validateProviderForm("onedrive", "root", { client_id: "client" }, oneDrive),
    "请先点击授权连接并完成 OneDrive 授权",
  );
  assert.equal(
    validateProviderForm("onedrive", "root", { client_id: "client", refresh_token: "refresh" }, oneDrive),
    "",
  );

  const google = defaultProviderFormOptions("googledrive", {}, false);
  assert.equal(
    validateProviderForm("googledrive", "root", { client_id: "client", client_secret: "secret" }, google),
    "请先点击授权连接并完成 Google Drive 授权",
  );
});