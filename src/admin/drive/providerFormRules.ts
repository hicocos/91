import type { Kind } from "./constants";

export type ProviderFieldLike = {
  key: string;
  label: string;
  required?: boolean;
  defaultValue?: string;
};

export type S3Preset = "aws" | "r2" | "minio" | "custom";
export type ProviderFormOptions = {
  advancedOpen: boolean;
  oneDriveTarget: "personal" | "sharepoint";
  webdavAuth: "basic" | "anonymous";
  s3Preset: S3Preset;
  configured: Record<string, boolean>;
};

const TARGET_PROVIDERS = new Set<Kind>(["onedrive", "googledrive", "webdav", "s3"]);

export function isGuidedProvider(kind: Kind): boolean {
  return TARGET_PROVIDERS.has(kind);
}

export function defaultProviderFormOptions(
  kind: Kind,
  credentials: Record<string, string>,
  isEdit: boolean,
  configured: Record<string, boolean> = {},
): ProviderFormOptions {
  const sharePoint = credentials.is_sharepoint === "true" || Boolean(credentials.site_id || credentials.drive_id);
  const anonymousWebDAV = credentials._webdav_auth === "anonymous" || (
    isEdit && kind === "webdav" && !credentials.username && !credentials.password && !configured.password
  );
  return {
    advancedOpen: false,
    oneDriveTarget: sharePoint ? "sharepoint" : "personal",
    webdavAuth: anonymousWebDAV ? "anonymous" : "basic",
    s3Preset: isS3Preset(credentials._s3_preset) ? credentials._s3_preset : inferS3Preset(credentials),
    configured,
  };
}

function isS3Preset(value: string | undefined): value is S3Preset {
  return value === "aws" || value === "r2" || value === "minio" || value === "custom";
}

export function storageCredentialsForSave(credentials: Record<string, string>): Record<string, string> {
  const next = { ...credentials };
  delete next._webdav_auth;
  delete next._s3_preset;
  delete next.root_prefix;
  return next;
}

export function partitionProviderFields<T extends ProviderFieldLike>(
  kind: Kind,
  fields: T[],
  options: ProviderFormOptions,
): { primary: T[]; advanced: T[] } {
  if (kind === "onedrive") {
    const primaryKeys = new Set(["client_id"]);
    const advancedKeys = new Set(["client_secret", "tenant", "refresh_token"]);
    if (options.oneDriveTarget === "sharepoint") {
      advancedKeys.add("site_id");
      advancedKeys.add("drive_id");
    }
    return selectFields(fields, primaryKeys, advancedKeys);
  }
  if (kind === "googledrive") {
    return selectFields(fields, new Set(["client_id", "client_secret"]), new Set(["refresh_token", "shared_drive_id"]));
  }
  if (kind === "webdav") {
    const primaryKeys = new Set(["base_url"]);
    if (options.webdavAuth === "basic") {
      primaryKeys.add("username");
      primaryKeys.add("password");
    }
    return selectFields(fields, primaryKeys, new Set());
  }
  if (kind === "s3") {
    const primaryKeys = new Set(["bucket", "access_key_id", "secret_access_key"]);
    if (options.s3Preset === "aws" || options.s3Preset === "custom") primaryKeys.add("region");
    if (options.s3Preset !== "aws") primaryKeys.add("endpoint");
    return selectFields(fields, primaryKeys, new Set(["session_token", "force_path_style"]));
  }
  return { primary: fields, advanced: [] };
}

function selectFields<T extends ProviderFieldLike>(fields: T[], primaryKeys: Set<string>, advancedKeys: Set<string>) {
  return {
    primary: fields.filter((field) => primaryKeys.has(field.key)),
    advanced: fields.filter((field) => advancedKeys.has(field.key)),
  };
}

export function inferS3Preset(credentials: Record<string, string>): S3Preset {
  const endpoint = (credentials.endpoint ?? "").toLowerCase();
  if (!endpoint) return "aws";
  if (endpoint.includes("r2.cloudflarestorage.com")) return "r2";
  if (credentials.force_path_style === "true") return "minio";
  return "custom";
}

export function applyS3Preset(preset: S3Preset, credentials: Record<string, string>): Record<string, string> {
  if (preset === "aws") {
    return { endpoint: "", region: "us-east-1", force_path_style: "false" };
  }
  if (preset === "r2") {
    return { endpoint: credentials.endpoint ?? "", region: "auto", force_path_style: "true" };
  }
  if (preset === "minio") {
    return { endpoint: credentials.endpoint ?? "", region: "us-east-1", force_path_style: "true" };
  }
  return {
    endpoint: credentials.endpoint ?? "",
    region: credentials.region || "us-east-1",
    force_path_style: credentials.force_path_style || "false",
  };
}

export function rootFieldPlacement(kind: Kind): "primary" | "advanced" | "legacy" | "hidden" {
  if (kind === "localstorage") return "hidden";
  if (kind === "s3") return "primary";
  if (kind === "onedrive" || kind === "googledrive" || kind === "webdav") return "advanced";
  return "legacy";
}

export function validateProviderForm(
  kind: Kind,
  rootID: string,
  credentials: Record<string, string>,
  options: ProviderFormOptions,
): string {
  const has = (key: string) => Boolean((credentials[key] ?? "").trim() || options.configured[key]);
  if (kind === "onedrive") {
    if (!has("client_id")) return "请填写Client ID";
    if (options.oneDriveTarget === "sharepoint" && !has("site_id")) return "请填写SharePoint Site ID";
    if (options.oneDriveTarget === "sharepoint" && !has("drive_id")) return "请填写SharePoint Drive ID";
    if (!has("refresh_token")) return "请先点击授权连接并完成 OneDrive 授权";
  }
  if (kind === "googledrive") {
    if (!has("client_id")) return "请填写Client ID";
    if (!has("client_secret")) return "请填写Client Secret";
    if (!has("refresh_token")) return "请先点击授权连接并完成 Google Drive 授权";
  }
  if (kind === "webdav") {
    if (!has("base_url")) return "请填写WebDAV地址";
    if (options.webdavAuth === "basic" && !has("username")) return "请填写WebDAV用户名";
    if (options.webdavAuth === "basic" && !has("password")) return "请填写WebDAV密码";
  }
  if (kind === "s3") {
    if (options.s3Preset !== "aws" && !has("endpoint")) return "请填写Endpoint";
    if (!has("region")) return "请填写Region";
    if (!has("bucket")) return "请填写Bucket";
    if (!has("access_key_id")) return "请填写Access Key ID";
    if (!has("secret_access_key")) return "请填写Secret Access Key";
    if (!rootID.trim()) return "请填写扫描目录前缀";
  }
  return "";
}