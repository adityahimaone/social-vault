import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";
import { Hash, Loader2, Lock, Plus, Sparkles, Trash2 } from "lucide-react";
import { api, ApiError, type NsfwStatus, type TaggingStatus } from "../api";
import type { Tag } from "../types";

type LayerChoice = "rules" | "jev" | "both";

const LAYER_OPTIONS: ReadonlyArray<{
  value: LayerChoice;
  label: string;
  hint: string;
}> = [
  {
    value: "rules",
    label: "Rules saja",
    hint: "Cepat, gratis, presisi ~94%",
  },
  {
    value: "jev",
    label: "Jev saja",
    hint: "Butuh key, ~1 detik/bookmark, threshold 0.8",
  },
  {
    value: "both",
    label: "Keduanya",
    hint: "Rules dulu, Jev untuk sisanya",
  },
];

function layersForChoice(choice: LayerChoice): string[] {
  if (choice === "rules") return ["rules"];
  if (choice === "jev") return ["jev"];
  return ["rules", "jev"];
}

interface TagsViewProps {
  tags: Tag[];
  onRefresh: () => Promise<void>;
  onFilterByTag: (tagName: string) => void;
  onNotify: (msg: string, tone: "success" | "error" | "info") => void;
  onNsfwStatusChange?: (status: NsfwStatus) => void;
}

export function TagsView({
  tags,
  onRefresh,
  onFilterByTag,
  onNotify,
  onNsfwStatusChange,
}: TagsViewProps) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [taggingStatus, setTaggingStatus] = useState<TaggingStatus | null>(null);
  const [statusLoading, setStatusLoading] = useState(true);
  const [layerChoice, setLayerChoice] = useState<LayerChoice>("both");
  const [backfilling, setBackfilling] = useState(false);
  const [nsfwStatus, setNsfwStatus] = useState<NsfwStatus | null>(null);
  const [nsfwPassword, setNsfwPassword] = useState("");
  const [nsfwBusy, setNsfwBusy] = useState(false);
  const nsfwInputRef = useRef<HTMLInputElement | null>(null);
  const onNsfwStatusChangeRef = useRef(onNsfwStatusChange);
  onNsfwStatusChangeRef.current = onNsfwStatusChange;

  const refreshNsfwStatus = useCallback(async (signal?: AbortSignal) => {
    try {
      const st = await api.nsfwStatus(signal);
      if (signal?.aborted) return;
      setNsfwStatus(st);
      onNsfwStatusChangeRef.current?.(st);
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
    }
  }, []);

  const refreshTaggingStatus = useCallback(async (signal?: AbortSignal) => {
    try {
      const st = await api.taggingStatus(signal);
      if (signal?.aborted) return;
      setTaggingStatus(st);
    } catch (err) {
      if (err instanceof DOMException && err.name === "AbortError") return;
    } finally {
      if (!signal?.aborted) {
        setStatusLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    const ctrl = new AbortController();
    void refreshTaggingStatus(ctrl.signal);
    void refreshNsfwStatus(ctrl.signal);
    return () => ctrl.abort();
  }, [refreshTaggingStatus, refreshNsfwStatus]);

  async function handleCreate(e: FormEvent) {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;
    setBusy(true);
    try {
      await api.createTag(trimmed);
      setName("");
      await onRefresh();
      onNotify(`Tag #${trimmed} ready`, "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to create tag", "error");
    } finally {
      setBusy(false);
    }
  }

  async function handleDelete(tag: Tag) {
    try {
      await api.deleteTag(tag.id);
      await onRefresh();
      await refreshTaggingStatus();
      onNotify(`Deleted tag #${tag.name}`, "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to delete tag", "error");
    }
  }

  const untaggedCount = taggingStatus?.untagged_count ?? 0;
  const isBackfillRunning = backfilling || Boolean(taggingStatus?.last_backfill?.running);

  async function handleBackfill() {
    if (isBackfillRunning || untaggedCount === 0) return;
    setBackfilling(true);
    try {
      const res = await api.taggingBackfill(layersForChoice(layerChoice));
      await onRefresh();
      const parts = [
        `rules_tagged: ${res.rules_tagged}`,
        `jev_tagged: ${res.jev_tagged}`,
        `queued: ${res.queued}`,
        `skipped: ${res.skipped}`,
      ];
      if (res.failed > 0) {
        parts.push(`failed: ${res.failed}`);
      }
      const anyTagged = res.rules_tagged + res.jev_tagged > 0;
      onNotify(`Backfill selesai (${parts.join(", ")})`, anyTagged ? "success" : "info");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Gagal menjalankan backfill", "error");
    } finally {
      setBackfilling(false);
      await refreshTaggingStatus();
    }
  }

  const showNsfwUnlock = Boolean(nsfwStatus?.configured && nsfwStatus.locked);
  const showNsfwLockButton = Boolean(nsfwStatus?.configured && !nsfwStatus.locked);

  async function handleUnlockNsfw(e: FormEvent) {
    e.preventDefault();
    if (!nsfwPassword || nsfwBusy) return;
    setNsfwBusy(true);
    try {
      await api.nsfwUnlock(nsfwPassword);
      setNsfwPassword("");
      await refreshNsfwStatus();
      await onRefresh();
      await refreshTaggingStatus();
      onNotify("Hidden tag unlocked", "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Password salah", "error");
    } finally {
      setNsfwBusy(false);
    }
  }

  async function handleLockNsfw() {
    if (nsfwBusy) return;
    setNsfwBusy(true);
    try {
      await api.nsfwLock();
      await refreshNsfwStatus();
      await onRefresh();
      await refreshTaggingStatus();
      onNotify("Hidden tag locked", "info");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to lock hidden tag", "error");
    } finally {
      setNsfwBusy(false);
    }
  }

  return (
    <section className="content-panel" aria-labelledby="tags-heading">
      <div className="panel-header">
        <div>
          <h1 id="tags-heading" className="panel-title">
            Tags
          </h1>
          <p className="panel-subtitle">
            Organize bookmarks with lightweight labels. Click any tag to filter your vault.
          </p>
        </div>
        <div className="panel-header-actions">
          {showNsfwLockButton && (
            <button
              type="button"
              className="btn btn-secondary btn-compact"
              onClick={handleLockNsfw}
              disabled={nsfwBusy}
            >
              <Lock size={13} aria-hidden="true" />
              Lock
            </button>
          )}
          <span className="panel-count-pill tabular-nums">{tags.length} tags</span>
        </div>
      </div>

      <form onSubmit={handleCreate} className="inline-form">
        <label className="sr-only" htmlFor="new-tag-name">
          New tag name
        </label>
        <input
          id="new-tag-name"
          type="text"
          className="input"
          placeholder="New tag name (e.g. threads, design, go)"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <button type="submit" className="btn btn-primary" disabled={busy || !name.trim()}>
          <Plus size={16} aria-hidden="true" />
          Add tag
        </button>
      </form>

      <div className="autotag-card" aria-labelledby="autotag-heading">
        <div className="autotag-header">
          <div>
            <h2 id="autotag-heading" className="autotag-title">
              Auto-tagging
            </h2>
            <p className="autotag-subtitle">
              Jalankan auto-tagging pada bookmark yang belum punya tag.
            </p>
          </div>
          <div className="autotag-badges">
            {taggingStatus && !taggingStatus.jev_enabled && (
              <span className="autotag-muted-pill">Jev tidak aktif</span>
            )}
            <span className="panel-count-pill tabular-nums">
              {statusLoading
                ? "Memuat status…"
                : `${untaggedCount} bookmark belum ber-tag`}
            </span>
          </div>
        </div>

        <div className="autotag-layers" role="radiogroup" aria-labelledby="autotag-heading">
          {LAYER_OPTIONS.map((opt) => {
            const selected = layerChoice === opt.value;
            return (
              <label
                key={opt.value}
                className={`autotag-option ${selected ? "is-selected" : ""}`}
              >
                <input
                  type="radio"
                  name="backfill-layer"
                  value={opt.value}
                  checked={selected}
                  disabled={isBackfillRunning}
                  onChange={() => setLayerChoice(opt.value)}
                />
                <span className="autotag-option-copy">
                  <span className="autotag-option-title">{opt.label}</span>
                  <span className="autotag-option-hint">{opt.hint}</span>
                </span>
              </label>
            );
          })}
        </div>

        <div className="autotag-actions">
          <button
            type="button"
            className="btn btn-secondary"
            disabled={statusLoading || isBackfillRunning || untaggedCount === 0}
            onClick={handleBackfill}
          >
            {isBackfillRunning ? (
              <>
                <Loader2 size={15} className="spin" aria-hidden="true" />
                Menjalankan backfill…
              </>
            ) : (
              <>
                <Sparkles size={15} aria-hidden="true" />
                Tag ulang yang belum ber-tag
              </>
            )}
          </button>
        </div>
      </div>

      {showNsfwUnlock && (
        <div className="autotag-card" aria-labelledby="nsfw-unlock-heading">
          <div className="autotag-header">
            <div>
              <h2 id="nsfw-unlock-heading" className="autotag-title nsfw-lock-title">
                <Lock size={14} aria-hidden="true" />
                <span>Hidden tag</span>
              </h2>
              <p className="autotag-subtitle">
                Enter password to reveal hidden tag and its bookmarks for this session.
              </p>
            </div>
          </div>
          <form onSubmit={handleUnlockNsfw} className="inline-form">
            <label className="sr-only" htmlFor="nsfw-unlock-password">
              Hidden tag password
            </label>
            <input
              ref={nsfwInputRef}
              id="nsfw-unlock-password"
              type="password"
              className="input"
              placeholder="Password..."
              autoComplete="current-password"
              value={nsfwPassword}
              onChange={(e) => setNsfwPassword(e.target.value)}
            />
            <button type="submit" className="btn btn-primary" disabled={nsfwBusy || !nsfwPassword}>
              <Lock size={15} aria-hidden="true" />
              Unlock
            </button>
          </form>
        </div>
      )}

      {tags.length === 0 && !showNsfwUnlock ? (
        <p className="panel-empty">
          No tags yet. Tags added to bookmarks appear here automatically.
        </p>
      ) : (
        <div className="grid-cards">
          {showNsfwUnlock && (
            <div className="entity-card">
              <button
                type="button"
                className="entity-card-btn"
                onClick={() => nsfwInputRef.current?.focus()}
              >
                <span className="entity-card-title">
                  <Lock size={15} aria-hidden="true" className="entity-card-icon" />
                  <span className="entity-card-name">Hidden tag</span>
                </span>
                <span className="entity-card-meta">Locked</span>
              </button>
            </div>
          )}
          {tags.map((tag) => (
            <div key={tag.id} className="entity-card">
              <button
                type="button"
                className="entity-card-btn"
                onClick={() => onFilterByTag(tag.name)}
              >
                <span className="entity-card-title">
                  <Hash size={15} aria-hidden="true" className="entity-card-icon" />
                  <span className="entity-card-name">{tag.name}</span>
                </span>
                <span className="entity-card-meta tabular-nums">
                  {tag.count ?? 0} bookmarks
                </span>
              </button>
              <button
                type="button"
                className="btn btn-ghost btn-icon btn-icon-sm"
                onClick={() => handleDelete(tag)}
                aria-label={`Delete tag ${tag.name}`}
              >
                <Trash2 size={15} aria-hidden="true" />
              </button>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
