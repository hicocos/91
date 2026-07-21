import { useId, useMemo, useState } from "react";
import { ChevronDown } from "lucide-react";
import { PasswordInput } from "../PasswordInput";
import type { StorageProviderManifest } from "../api";
import { QuarkQRCodeLogin } from "./QuarkQRCodeLogin";
import { P115QRCodeLogin } from "./P115QRCodeLogin";
import { P123QRCodeLogin } from "./P123QRCodeLogin";
import { WopanQRCodeLogin } from "./WopanQRCodeLogin";
import { GuangYaPanQRCodeLogin } from "./GuangYaPanQRCodeLogin";
import {
  FormState,
  Kind,
  credentialFields,
  driveKindIconPath,
  rootDirectoryLabel,
  rootIdPlaceholder,
  usesRootDirectoryID,
} from "./constants";
import {
  applyS3Preset,
  defaultProviderFormOptions,
  isGuidedProvider,
  partitionProviderFields,
  rootFieldPlacement,
  type S3Preset,
} from "./providerFormRules";

type DriveOption = {
  kind: Kind;
  label: string;
  abbr: string;
};

const DRIVE_OPTIONS: DriveOption[] = [
  { kind: "p115", label: "115 网盘", abbr: "115" },
  { kind: "p123", label: "123网盘", abbr: "123" },
  { kind: "pikpak", label: "PikPak", abbr: "Pk" },
  { kind: "guangyapan", label: "光鸭网盘", abbr: "GY" },
  { kind: "onedrive", label: "OneDrive", abbr: "OD" },
  { kind: "googledrive", label: "Google Drive", abbr: "GD" },
  { kind: "quark", label: "夸克网盘", abbr: "Qk" },
  { kind: "wopan", label: "联通网盘", abbr: "Wo" },
  { kind: "webdav", label: "WebDAV", abbr: "WD" },
  { kind: "s3", label: "S3 兼容存储", abbr: "S3" },
  { kind: "localstorage", label: "本地存储", abbr: "Lo" },
];

export function DriveForm({
  form,
  onChange,
  isEdit,
  onTypeSelected,
  providerManifest,
}: {
  form: FormState;
  onChange: (f: FormState) => void;
  isEdit: boolean;
  onTypeSelected?: () => void;
  providerManifest?: StorageProviderManifest;
}) {
  const idPrefix = useId();
  const fields = useMemo(() => providerManifest && !providerManifest.legacy
    ? providerManifest.fields.map((field) => ({
        key: field.key,
        label: field.label,
        placeholder: field.help ?? "",
        type: field.type === "boolean" ? "select" as const : "text" as const,
        options: field.type === "boolean" ? [{ value: "false", label: "关闭" }, { value: "true", label: "开启" }] : undefined,
        required: field.required,
        defaultValue: field.defaultValue,
        multiline: false,
      }))
    : credentialFields(form.kind), [form.kind, providerManifest]);
  const [step, setStep] = useState<"type" | "form">(isEdit ? "form" : "type");
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const nameId = `${idPrefix}-drive-name`;
  const rootId = `${idPrefix}-drive-root`;
  const options = useMemo(
    () => defaultProviderFormOptions(form.kind, form.creds, isEdit, form.configured),
    [form.kind, form.creds, form.configured, isEdit],
  );
  const groupedFields = useMemo(
    () => partitionProviderFields(form.kind, fields, { ...options, advancedOpen }),
    [form.kind, fields, options, advancedOpen],
  );
  const rootPlacement = rootFieldPlacement(form.kind);

  function set<K extends keyof FormState>(k: K, v: FormState[K]) {
    onChange({ ...form, [k]: v });
  }
  function setCred(k: string, v: string) {
    onChange({ ...form, creds: { ...form.creds, [k]: v } });
  }
  function setKind(v: Kind) {
    const initialCreds: Record<string, string> = v === "s3"
      ? { region: "us-east-1", force_path_style: "false", _s3_preset: "aws" }
      : v === "webdav"
        ? { _webdav_auth: "basic" }
        : v === "pikpak"
          ? { platform: "web", disable_media_link: "true" }
          : {};
    onChange({
      ...form,
      kind: v,
      rootId: "",
      creds: initialCreds,
    });
    setAdvancedOpen(false);
  }
  function selectType(kind: Kind) {
    setKind(kind);
    setStep("form");
    onTypeSelected?.();
  }

  const selectedOption = DRIVE_OPTIONS.find((o) => o.kind === form.kind);
  const selectedIconSrc = selectedOption ? driveKindIconPath(selectedOption.kind) : "";

  function setOneDriveTarget(value: "personal" | "sharepoint") {
    onChange({
      ...form,
      creds: {
        ...form.creds,
        is_sharepoint: value === "sharepoint" ? "true" : "false",
      },
    });
  }

  function setWebDAVAuth(value: "basic" | "anonymous") {
    onChange({
      ...form,
      creds: {
        ...form.creds,
        _webdav_auth: value,
        ...(value === "anonymous" ? { username: "", password: "" } : {}),
      },
    });
  }

  function setS3Preset(value: S3Preset) {
    onChange({
      ...form,
      creds: {
        ...form.creds,
        ...applyS3Preset(value, form.creds),
        _s3_preset: value,
      },
    });
  }

  function renderField(f: (typeof fields)[number]) {
    const fieldId = `${idPrefix}-credential-${f.key}`;
    return (
      <div key={f.key} className="admin-form__row">
        <label htmlFor={fieldId}>
          {f.label}
          {isEdit && isSecretCredential(f.key) && form.configured?.[f.key] && !form.creds[f.key]
            ? "（已配置，留空则保持不变）"
            : ""}
        </label>
        {f.type === "select" ? (
          <div className="admin-form-select-wrap">
            <select
              id={fieldId}
              className="admin-form-select"
              value={form.creds[f.key] ?? f.defaultValue ?? ""}
              onChange={(e) => setCred(f.key, e.target.value)}
            >
              {(f.options ?? []).map((option) => (
                <option key={option.value} value={option.value}>{option.label}</option>
              ))}
            </select>
            <ChevronDown size={15} className="admin-form-select__icon" aria-hidden="true" />
          </div>
        ) : f.multiline ? (
          <textarea
            id={fieldId}
            placeholder={f.placeholder}
            value={form.creds[f.key] ?? ""}
            onChange={(e) => setCred(f.key, e.target.value)}
          />
        ) : isSecretCredential(f.key) ? (
          <PasswordInput
            id={fieldId}
            placeholder={f.placeholder}
            value={form.creds[f.key] ?? ""}
            onChange={(e) => setCred(f.key, e.target.value)}
          />
        ) : (
          <input
            id={fieldId}
            type="text"
            placeholder={f.placeholder}
            value={form.creds[f.key] ?? f.defaultValue ?? ""}
            onChange={(e) => setCred(f.key, e.target.value)}
          />
        )}
      </div>
    );
  }

  function renderRootField() {
    if (!usesRootDirectoryID(form.kind)) return null;
    return (
      <div className="admin-form__row admin-provider-root-field">
        <label htmlFor={rootId}>{rootDirectoryLabel(form.kind)}</label>
        <input
          id={rootId}
          placeholder={rootIdPlaceholder(form.kind)}
          value={form.rootId}
          onChange={(e) => set("rootId", e.target.value)}
        />
      </div>
    );
  }

  if (step === "type" && !isEdit) {
    return (
      <div className="admin-drive-type-picker">
        <div className="admin-drive-type-grid">
          {DRIVE_OPTIONS.map((opt) => {
            const iconSrc = driveKindIconPath(opt.kind);
            return (
              <button
                key={opt.kind}
                type="button"
                className="admin-drive-type-card"
                data-kind={opt.kind}
                onClick={() => selectType(opt.kind)}
              >
                <span
                  className={`admin-drive-type-card__icon${iconSrc ? " has-image" : ""}`}
                  data-kind={opt.kind}
                >
                  {iconSrc ? (
                    <img
                      src={iconSrc}
                      alt=""
                      aria-hidden="true"
                      className="admin-drive-type-card__icon-img"
                    />
                  ) : (
                    opt.abbr
                  )}
                </span>
                <span className="admin-drive-type-card__label">{opt.label}</span>
              </button>
            );
          })}
        </div>
      </div>
    );
  }

  return (
    <div className="admin-form">
      {!isEdit && selectedOption && (
        <div className="admin-drive-selected-bar" data-kind={form.kind}>
          <span
            className={`admin-drive-selected-bar__icon${selectedIconSrc ? " has-image" : ""}`}
            data-kind={form.kind}
          >
            {selectedIconSrc ? (
              <img
                src={selectedIconSrc}
                alt=""
                aria-hidden="true"
                className="admin-drive-selected-bar__icon-img"
              />
            ) : (
              selectedOption.abbr
            )}
          </span>
          <div className="admin-drive-selected-bar__text">
            <span className="admin-drive-selected-bar__name">{selectedOption.label}</span>
          </div>
        </div>
      )}

      <div className="admin-form__section">
        <div className="admin-form__row">
          <label htmlFor={nameId}>名称</label>
          <input
            id={nameId}
            value={form.name}
            onChange={(e) => set("name", e.target.value)}
          />
        </div>
      </div>

      {fields.length > 0 && (
        <div className="admin-form__section">
          {isGuidedProvider(form.kind) && (
            <div className="admin-provider-guided-controls">
              {form.kind === "onedrive" && (
                <div className="admin-provider-mode" role="group" aria-label="OneDrive 类型">
                  <button type="button" className={options.oneDriveTarget === "personal" ? "is-active" : ""} onClick={() => setOneDriveTarget("personal")}>个人 OneDrive</button>
                  <button type="button" className={options.oneDriveTarget === "sharepoint" ? "is-active" : ""} onClick={() => setOneDriveTarget("sharepoint")}>SharePoint</button>
                </div>
              )}
              {form.kind === "webdav" && (
                <div className="admin-provider-mode" role="group" aria-label="WebDAV 认证方式">
                  <button type="button" className={options.webdavAuth === "basic" ? "is-active" : ""} onClick={() => setWebDAVAuth("basic")}>账号密码</button>
                  <button type="button" className={options.webdavAuth === "anonymous" ? "is-active" : ""} onClick={() => setWebDAVAuth("anonymous")}>匿名访问</button>
                </div>
              )}
              {form.kind === "s3" && (
                <div className="admin-form__row">
                  <label htmlFor={`${idPrefix}-s3-preset`}>服务类型</label>
                  <div className="admin-form-select-wrap">
                    <select id={`${idPrefix}-s3-preset`} className="admin-form-select" value={options.s3Preset} onChange={(e) => setS3Preset(e.target.value as S3Preset)}>
                      <option value="aws">AWS S3</option>
                      <option value="r2">Cloudflare R2</option>
                      <option value="minio">MinIO / 私有 S3</option>
                      <option value="custom">其他兼容服务</option>
                    </select>
                    <ChevronDown size={15} className="admin-form-select__icon" aria-hidden="true" />
                  </div>
                </div>
              )}
            </div>
          )}

          {form.kind === "quark" && (
            <QuarkQRCodeLogin onCookie={(cookie) => setCred("cookie", cookie)} />
          )}
          {form.kind === "p115" && (
            <P115QRCodeLogin onCookie={(cookie) => setCred("cookie", cookie)} />
          )}
          {form.kind === "p123" && (
            <P123QRCodeLogin onToken={(token) => setCred("access_token", token)} />
          )}
          {form.kind === "wopan" && (
            <WopanQRCodeLogin
              onCredentials={(credentials) => onChange({
                ...form,
                creds: {
                  ...form.creds,
                  access_token: credentials.accessToken,
                  refresh_token: credentials.refreshToken,
                  ...(credentials.familyID ? { family_id: credentials.familyID } : {}),
                },
              })}
            />
          )}
          {form.kind === "guangyapan" && (
            <GuangYaPanQRCodeLogin
              onCredentials={(credentials) => onChange({
                ...form,
                creds: {
                  ...form.creds,
                  access_token: credentials.accessToken,
                  refresh_token: credentials.refreshToken,
                },
              })}
            />
          )}

          {form.kind === "p123" && <div className="admin-form__method-label">方式二</div>}
          {form.kind === "p115" && <div className="admin-form__method-label">方式二</div>}
          {form.kind === "quark" && <div className="admin-form__method-label">方式二</div>}

          {(isGuidedProvider(form.kind) ? groupedFields.primary : fields).map(renderField)}

          {rootPlacement === "primary" && renderRootField()}

          {(form.kind === "onedrive" || form.kind === "googledrive") && (
            <button type="button" className="admin-btn is-primary admin-provider-oauth" onClick={() => window.dispatchEvent(new CustomEvent("storage-oauth-start", { detail: { provider: form.kind } }))}>
              授权连接
            </button>
          )}

          {form.kind === "googledrive" && (
            <div className="admin-mount-docs__notice is-info admin-google-drive-scope-help">
              <div>
                <strong>Google Drive 挂载范围说明</strong>
                <ul>
                  <li>个人盘：团队盘 ID 留空；扫描起点留空表示扫描整个“我的云端硬盘”。</li>
                  <li>整个团队盘：填写“共享云端硬盘（团队盘）ID”，扫描起点留空，无需重复填写。</li>
                  <li>团队盘中的子文件夹：同时填写团队盘 ID，并在扫描起点填写子文件夹 ID。</li>
                </ul>
              </div>
            </div>
          )}

          {isGuidedProvider(form.kind) && (groupedFields.advanced.length > 0 || rootPlacement === "advanced") && (
            <div className="admin-provider-advanced">
              <button type="button" className="admin-provider-advanced__toggle" aria-expanded={advancedOpen} onClick={() => setAdvancedOpen((open) => !open)}>
                <span>高级设置</span>
                <ChevronDown size={15} aria-hidden="true" />
              </button>
              {advancedOpen && (
                <div className="admin-provider-advanced__body">
                  {groupedFields.advanced.map(renderField)}
                  {rootPlacement === "advanced" && renderRootField()}
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {rootPlacement === "legacy" && (
        <div className="admin-form__section">{renderRootField()}</div>
      )}
    </div>
  );
}

function isSecretCredential(key: string): boolean {
  return /password|token|secret/i.test(key);
}
