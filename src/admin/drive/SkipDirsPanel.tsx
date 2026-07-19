import { useCallback, useEffect, useState } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import * as api from "../api";
import { useToast } from "../ToastContext";
import {
  nextDirTreeLoadState,
  type DirTreeLoadState,
} from "./dirTreeLoadState";

type SkipDirsPanelProps = {
  drive: api.AdminDrive;
  onSaved: (saved: { id: string; skipDirIds: string[] }) => void;
};

export function SkipDirsPanel({ drive, onSaved }: SkipDirsPanelProps) {
  const { show } = useToast();
  const [selected, setSelected] = useState<Set<string>>(
    () => new Set(drive.skipDirIds ?? [])
  );
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    setSelected(new Set(drive.skipDirIds ?? []));
  }, [drive.id, drive.skipDirIds]);

  const toggle = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  async function handleSave() {
    setSaving(true);
    try {
      const ids = Array.from(selected);
      const resp = await api.setDriveSkipDirIds(drive.id, ids);
      onSaved({ id: drive.id, skipDirIds: resp.skipDirIds });
    } catch (e) {
      show(e instanceof Error ? e.message : "保存失败", "error");
    } finally {
      setSaving(false);
    }
  }

  return (
    <div className="admin-detail-card">
      <header className="admin-detail-card__title">
        <div className="admin-detail-card__title-left">
          <span>扫描跳过目录</span>
        </div>
        <button
          className="admin-btn"
          onClick={handleSave}
          disabled={saving}
          style={{ padding: "4px 10px", fontSize: "12px", height: "auto" }}
        >
          {saving ? "保存中..." : "保存更改"}
        </button>
      </header>

      <div style={{ display: "flex", flexDirection: "column", gap: "12px" }}>
        <div className="admin-detail-tree-container">
          <DirTreeNode
            driveId={drive.id}
            id=""
            name={drive.name || "存储"}
            depth={0}
            initiallyOpen
            ancestorSkipped={false}
            selected={selected}
            onToggle={toggle}
          />
        </div>
      </div>
    </div>
  );
}

type DirTreeNodeProps = {
  driveId: string;
  id: string;
  name: string;
  depth: number;
  initiallyOpen?: boolean;
  ancestorSkipped: boolean;
  selected: Set<string>;
  onToggle: (id: string) => void;
};

function DirTreeNode({
  driveId,
  id,
  name,
  depth,
  initiallyOpen,
  ancestorSkipped,
  selected,
  onToggle,
}: DirTreeNodeProps) {
  const [open, setOpen] = useState(!!initiallyOpen);
  const [loadState, setLoadState] = useState<DirTreeLoadState>("idle");
  const [children, setChildren] = useState<api.DriveDirEntry[]>([]);
  const [error, setError] = useState("");

  const isRoot = depth === 0;
  const isSelected = id !== "" && selected.has(id);
  const dimmed = ancestorSkipped;
  const loading = loadState === "loading";
  const loaded = loadState === "loaded";

  useEffect(() => {
    if (!open || loadState !== "idle") return;

    let active = true;
    setLoadState((state) => nextDirTreeLoadState(state, "start"));
    setError("");

    void api
      .listDriveDirChildren(driveId, id || undefined)
      .then((data) => {
        if (!active) return;
        setChildren(data ?? []);
        setLoadState((state) => nextDirTreeLoadState(state, "resolve"));
      })
      .catch((e: unknown) => {
        if (!active) return;
        setError(e instanceof Error ? e.message : "加载失败");
        setLoadState((state) => nextDirTreeLoadState(state, "reject"));
      });

    return () => {
      active = false;
    };
  }, [driveId, id, loadState, open]);

  function handleToggleOpen() {
    setOpen((v) => !v);
  }

  function handleRetry() {
    setLoadState((state) => nextDirTreeLoadState(state, "retry"));
  }

  return (
    <div
      style={{
        paddingLeft: depth <= 1 ? 0 : 16,
        opacity: dimmed && !isSelected ? 0.55 : 1,
      }}
    >
      {!isRoot && (
        <div
          style={{
            display: "flex",
            alignItems: "center",
            gap: "6px",
            padding: "4px 6px",
            borderRadius: "4px",
            background: isSelected ? "var(--accent-soft, rgba(255,140,0,0.12))" : "transparent",
          }}
        >
          <button
            type="button"
            onClick={handleToggleOpen}
            style={{
              border: "none",
              background: "transparent",
              cursor: "pointer",
              padding: 0,
              display: "inline-flex",
              alignItems: "center",
            }}
            aria-label={open ? "折叠" : "展开"}
          >
            {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
          </button>

          <input
            type="checkbox"
            checked={isSelected}
            onChange={() => onToggle(id)}
            aria-label={`跳过目录 ${name}`}
          />

          <span
            style={{
              fontSize: "13px",
              cursor: "pointer",
              userSelect: "none",
              fontWeight: 400,
            }}
            onClick={handleToggleOpen}
          >
            {name}
          </span>
        </div>
      )}

      {open && (
        <div>
          {loading && (
            <div className="admin-text-faint" style={{ fontSize: "12px", padding: "4px 28px" }}>
              加载中...
            </div>
          )}
          {error && (
            <div style={{ fontSize: "12px", padding: "4px 28px", color: "var(--danger, #d33)" }}>
              <span>{error}</span>{" "}
              <button type="button" className="admin-btn" onClick={handleRetry}>
                重试
              </button>
            </div>
          )}
          {loaded && !error && children.length === 0 && (
            <div className="admin-text-faint" style={{ fontSize: "12px", padding: "4px 28px" }}>
              （无子目录）
            </div>
          )}
          {children.map((child) => (
            <DirTreeNode
              key={child.id}
              driveId={driveId}
              id={child.id}
              name={child.name}
              depth={depth + 1}
              ancestorSkipped={ancestorSkipped || isSelected}
              selected={selected}
              onToggle={onToggle}
            />
          ))}
        </div>
      )}
    </div>
  );
}
