import { useEffect, useRef, useState } from "react";
import {
  Archive,
  ArchiveRestore,
  Check,
  Copy,
  ExternalLink,
  Eye,
  FolderKanban,
  Globe,
  Hash,
  Layers,
  Sparkles,
  Star,
  Trash2,
  X,
} from "lucide-react";
import { api, ApiError } from "../api";
import type { Bookmark, List, Stats } from "../types";
import { buildSimilarQuery, domainFromURL, formatDateTime } from "../utils/bookmarks";
import { useFocusTrap } from "./glass/useFocusTrap";

interface BookmarkInspectorProps {
  bookmark: Bookmark | null;
  open: boolean;
  isMobileViewport: boolean;
  isFavorite: boolean;
  allLists: List[];
  stats: Stats | null;
  withThumbnailCount: number;
  onClose: () => void;
  onSelectBookmark: (bookmark: Bookmark) => void;
  onSelectTagFilter: (tagName: string) => void;
  onToggleFavorite: (bookmark: Bookmark) => void;
  onToggleArchive: (bookmark: Bookmark) => Promise<void>;
  onQuickLook: (bookmark: Bookmark) => void;
  onUpdated: (bookmark: Bookmark) => void;
  onDeleted: (id: number) => void;
  onNotify: (msg: string, tone: "success" | "error") => void;
}

function SimilarThumb({ item }: { item: Bookmark }) {
  const [failed, setFailed] = useState(false);
  const domain = item.site_name || domainFromURL(item.url);

  useEffect(() => {
    setFailed(false);
  }, [item.image_url]);

  if (item.image_url && !failed) {
    return (
      <div className="similar-thumb">
        <img
          src={item.image_url}
          alt={item.title || domain}
          loading="lazy"
          onError={() => setFailed(true)}
        />
      </div>
    );
  }

  return (
    <div className="similar-thumb similar-thumb-fallback" aria-hidden="true">
      <Globe size={14} />
    </div>
  );
}

export function BookmarkInspector({
  bookmark,
  open,
  isMobileViewport,
  isFavorite,
  allLists,
  stats,
  withThumbnailCount,
  onClose,
  onSelectBookmark,
  onSelectTagFilter,
  onToggleFavorite,
  onToggleArchive,
  onQuickLook,
  onUpdated,
  onDeleted,
  onNotify,
}: BookmarkInspectorProps) {
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [notes, setNotes] = useState("");
  const [tags, setTags] = useState<string[]>([]);
  const [tagDraft, setTagDraft] = useState("");
  const [selectedListIDs, setSelectedListIDs] = useState<number[]>([]);
  const [saving, setSaving] = useState(false);
  const [copied, setCopied] = useState(false);
  const [previewFailed, setPreviewFailed] = useState(false);
  const [dimensions, setDimensions] = useState<{ w: number; h: number } | null>(null);

  const [similar, setSimilar] = useState<Bookmark[]>([]);
  const [loadingSimilar, setLoadingSimilar] = useState(false);

  const [holding, setHolding] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const holdTimerRef = useRef<number | null>(null);
  const panelRef = useRef<HTMLElement | null>(null);

  const useModalDrawer = isMobileViewport && Boolean(bookmark);
  useFocusTrap(panelRef as React.RefObject<HTMLDivElement | null>, useModalDrawer && open);

  useEffect(() => {
    if (!bookmark) return;
    setTitle(bookmark.title);
    setDescription(bookmark.description);
    setNotes(bookmark.notes);
    setTags(bookmark.tags.map((t) => t.name));
    setSelectedListIDs(bookmark.lists.map((l) => l.id));
    setTagDraft("");
    setConfirmDelete(false);
    setHolding(false);
    setCopied(false);
    setPreviewFailed(false);
    setDimensions(null);
  }, [bookmark]);

  useEffect(() => {
    if (!bookmark || !open) {
      setSimilar([]);
      return;
    }

    const ctrl = new AbortController();
    const query = buildSimilarQuery(bookmark);
    const firstTag = bookmark.tags[0]?.name ?? "";

    async function loadSimilar() {
      setLoadingSimilar(true);
      try {
        const seen = new Set<number>([bookmark!.id]);
        const collected: Bookmark[] = [];

        if (query) {
          const res = await api.listBookmarks({ q: query, limit: 8 }, ctrl.signal);
          for (const item of res.items) {
            if (!seen.has(item.id)) {
              seen.add(item.id);
              collected.push(item);
            }
          }
        }

        if (collected.length < 4 && firstTag) {
          const tagRes = await api.listBookmarks({ tag: firstTag, limit: 8 }, ctrl.signal);
          for (const item of tagRes.items) {
            if (!seen.has(item.id)) {
              seen.add(item.id);
              collected.push(item);
            }
          }
        }

        setSimilar(collected.slice(0, 6));
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setSimilar([]);
      } finally {
        if (!ctrl.signal.aborted) setLoadingSimilar(false);
      }
    }

    void loadSimilar();
    return () => ctrl.abort();
  }, [bookmark, open]);

  useEffect(() => {
    if (!useModalDrawer || !open) return;
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        onClose();
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [useModalDrawer, open, onClose]);

  if (!open) return null;
  if (isMobileViewport && !bookmark) return null;

  function addTagFromDraft() {
    const clean = tagDraft.trim();
    if (!clean) return;
    if (!tags.some((t) => t.toLowerCase() === clean.toLowerCase())) {
      setTags([...tags, clean]);
    }
    setTagDraft("");
  }

  function removeTag(name: string) {
    setTags(tags.filter((t) => t !== name));
  }

  function toggleList(id: number) {
    setSelectedListIDs((prev) =>
      prev.includes(id) ? prev.filter((item) => item !== id) : [...prev, id]
    );
  }

  async function handleCopyURL(raw: string) {
    try {
      await navigator.clipboard.writeText(raw);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      onNotify("Could not copy URL", "error");
    }
  }

  async function handleSave() {
    if (!bookmark) return;
    setSaving(true);
    try {
      const updated = await api.updateBookmark(bookmark.id, {
        title,
        description,
        notes,
        tags,
        list_ids: selectedListIDs,
      });
      onUpdated(updated);
      onNotify("Bookmark saved", "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to save bookmark", "error");
    } finally {
      setSaving(false);
    }
  }

  async function handleArchiveClick() {
    if (!bookmark) return;
    setSaving(true);
    try {
      await onToggleArchive(bookmark);
    } finally {
      setSaving(false);
    }
  }

  async function executeDelete() {
    if (!bookmark) return;
    try {
      await api.deleteBookmark(bookmark.id);
      onDeleted(bookmark.id);
      onNotify("Bookmark deleted", "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to delete bookmark", "error");
    }
  }

  function startHoldDelete() {
    setHolding(true);
    holdTimerRef.current = window.setTimeout(() => {
      setHolding(false);
      void executeDelete();
    }, 1400);
  }

  function cancelHoldDelete() {
    setHolding(false);
    if (holdTimerRef.current !== null) {
      window.clearTimeout(holdTimerRef.current);
      holdTimerRef.current = null;
    }
  }

  const active = bookmark;
  const domain = active ? active.site_name || domainFromURL(active.url) : "";

  const isDirty = active
    ? title !== active.title ||
      description !== active.description ||
      notes !== active.notes ||
      tags.join("|") !== active.tags.map((t) => t.name).join("|") ||
      selectedListIDs.slice().sort().join(",") !==
        active.lists.map((l) => l.id).slice().sort().join(",")
    : false;

  const total = stats?.total ?? 0;
  const activeCount = stats?.active ?? 0;
  const activeRatio = total > 0 ? Math.round((activeCount / total) * 100) : 0;

  const panelContent = (
    <aside
      ref={panelRef}
      className={`inspector-panel ${useModalDrawer ? "is-drawer" : ""}`.trim()}
      aria-label={active ? "Bookmark info inspector" : "Vault overview"}
      role={useModalDrawer ? "dialog" : "complementary"}
      aria-modal={useModalDrawer ? "true" : undefined}
    >
      <div className="inspector-header">
        <div className="drawer-title-wrap">
          <h2 className="drawer-title">{active ? "Info" : "Vault Info"}</h2>
          {active?.archived && <span className="status-badge">Archived</span>}
        </div>

        <div className="drawer-header-actions">
          {active && (
            <>
              <button
                type="button"
                className="btn btn-ghost btn-icon btn-icon-sm"
                onClick={() => onQuickLook(active)}
                aria-label="Quick Look"
                title="Quick Look (Space)"
              >
                <Eye size={15} aria-hidden="true" />
              </button>
              <button
                type="button"
                className={`btn btn-ghost btn-icon btn-icon-sm ${isFavorite ? "is-fav" : ""}`}
                onClick={() => onToggleFavorite(active)}
                aria-label={isFavorite ? "Remove from favourites" : "Add to favourites"}
                aria-pressed={isFavorite}
                title={isFavorite ? "Unfavourite" : "Favourite"}
              >
                <Star size={15} aria-hidden="true" fill={isFavorite ? "currentColor" : "none"} />
              </button>
              <a
                href={active.url}
                target="_blank"
                rel="noopener noreferrer"
                className="btn btn-ghost btn-icon btn-icon-sm"
                aria-label="Open URL in new tab"
                title="Open URL"
              >
                <ExternalLink size={15} aria-hidden="true" />
              </a>
            </>
          )}
          <button
            type="button"
            className="btn btn-ghost btn-icon btn-icon-sm"
            onClick={onClose}
            aria-label="Close info panel"
            title="Close info panel"
          >
            <X size={16} aria-hidden="true" />
          </button>
        </div>
      </div>

      {!active ? (
        <div className="inspector-body inspector-empty-overview">
          <div className="overview-hero">
            <div className="overview-hero-top">
              <span className="stat-icon-well" aria-hidden="true">
                <Layers size={14} />
              </span>
              <span className="overview-label">Library Snapshot</span>
            </div>
            <div className="overview-number tabular-nums">{stats ? total : "—"}</div>
            <div className="stat-meter" aria-hidden="true">
              <span className="stat-meter-fill" style={{ width: `${activeRatio}%` }} />
            </div>
            <div className="overview-sub tabular-nums">
              {stats
                ? `${activeCount} active · ${stats.archived} archived · ${withThumbnailCount} previews`
                : "Loading stats..."}
            </div>
          </div>

          <div className="inspector-hint-box">
            <Sparkles size={15} aria-hidden="true" />
            <div>
              <div className="inspector-hint-title">Select any reference</div>
              <p className="inspector-hint-copy">
                Click a card or press <kbd>J</kbd>/<kbd>K</kbd> to inspect metadata, edit notes &amp; tags, and press <kbd>Space</kbd> for Quick Look.
              </p>
            </div>
          </div>

          {stats && stats.top_tags.length > 0 && (
            <div className="inspector-section">
              <div className="inspector-section-title">Quick tag filters</div>
              <div className="pill-row">
                {stats.top_tags.map((tag) => (
                  <button
                    key={tag.id}
                    type="button"
                    className="tag-pill"
                    onClick={() => onSelectTagFilter(tag.name)}
                  >
                    <Hash size={11} aria-hidden="true" />
                    <span>{tag.name}</span>
                    <span className="stat-tag-count tabular-nums">{tag.count ?? 0}</span>
                  </button>
                ))}
              </div>
            </div>
          )}
        </div>
      ) : (
        <>
          <div className="inspector-body">
            {active.image_url && !previewFailed && (
              <div className="inspector-preview">
                <img
                  src={active.image_url}
                  alt={active.title || domain}
                  onLoad={(e) => {
                    const img = e.currentTarget;
                    if (img.naturalWidth > 0 && img.naturalHeight > 0) {
                      setDimensions({ w: img.naturalWidth, h: img.naturalHeight });
                    }
                  }}
                  onError={() => setPreviewFailed(true)}
                />
              </div>
            )}

            <div className="inspector-spec-grid">
              <div className="spec-cell">
                <span className="spec-label">Source</span>
                <span className="spec-value">{domain}</span>
              </div>
              <div className="spec-cell">
                <span className="spec-label">Preview</span>
                <span className="spec-value tabular-nums">
                  {dimensions
                    ? `${dimensions.w} × ${dimensions.h}`
                    : active.image_url && !previewFailed
                      ? "Loaded"
                      : "Text card"}
                </span>
              </div>
              <div className="spec-cell">
                <span className="spec-label">Added</span>
                <span className="spec-value tabular-nums">{formatDateTime(active.created_at)}</span>
              </div>
              <div className="spec-cell">
                <span className="spec-label">Ingest</span>
                <span className="spec-value">{active.source || "manual"}</span>
              </div>
            </div>

            <div className="inspector-url-row">
              <a href={active.url} target="_blank" rel="noopener noreferrer" className="inspector-url-link">
                {active.url}
              </a>
              <button
                type="button"
                className="btn btn-ghost btn-icon btn-icon-sm"
                onClick={() => handleCopyURL(active.url)}
                aria-label="Copy URL"
                title="Copy URL"
              >
                {copied ? <Check size={14} aria-hidden="true" /> : <Copy size={14} aria-hidden="true" />}
              </button>
            </div>

            <div className="field">
              <label className="field-label" htmlFor="inspector-title">
                Title
              </label>
              <input
                id="inspector-title"
                type="text"
                className="input"
                value={title}
                onChange={(e) => setTitle(e.target.value)}
              />
            </div>

            <div className="field">
              <label className="field-label" htmlFor="inspector-desc">
                Description
              </label>
              <textarea
                id="inspector-desc"
                className="textarea textarea-compact"
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            </div>

            <div className="field">
              <label className="field-label" htmlFor="inspector-notes">
                Notes
              </label>
              <textarea
                id="inspector-notes"
                className="textarea textarea-compact"
                placeholder="Why you saved this reference..."
                value={notes}
                onChange={(e) => setNotes(e.target.value)}
              />
            </div>

            <div className="field">
              <label className="field-label" htmlFor="inspector-tag-input">
                Tags
              </label>
              <div className="tag-input-row">
                <input
                  id="inspector-tag-input"
                  type="text"
                  className="input"
                  placeholder="Add tag + Enter"
                  value={tagDraft}
                  onChange={(e) => setTagDraft(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      addTagFromDraft();
                    }
                  }}
                />
                <button type="button" className="btn btn-secondary btn-compact" onClick={addTagFromDraft}>
                  Add
                </button>
              </div>
              {tags.length > 0 && (
                <div className="pill-row">
                  {tags.map((tag) => (
                    <button
                      key={tag}
                      type="button"
                      className="tag-pill"
                      onClick={() => removeTag(tag)}
                      aria-label={`Remove tag ${tag}`}
                    >
                      <Hash size={11} aria-hidden="true" />
                      {tag}
                      <X size={11} aria-hidden="true" />
                    </button>
                  ))}
                </div>
              )}
            </div>

            {allLists.some((l) => !l.is_smart) && (
              <div className="field">
                <span className="field-label">Lists</span>
                <div className="pill-row">
                  {allLists
                    .filter((list) => !list.is_smart)
                    .map((list) => {
                      const checked = selectedListIDs.includes(list.id);
                      return (
                        <button
                          key={list.id}
                          type="button"
                          className={checked ? "list-pill" : "tag-pill"}
                          aria-pressed={checked}
                          onClick={() => toggleList(list.id)}
                        >
                          <FolderKanban size={11} aria-hidden="true" />
                          {list.name}
                        </button>
                      );
                    })}
                </div>
              </div>
            )}

            <div className="inspector-section">
              <div className="inspector-section-head">
                <span className="inspector-section-title">Similar references</span>
                {loadingSimilar && <span className="inspector-section-meta">Searching...</span>}
              </div>
              {similar.length > 0 ? (
                <div className="similar-grid">
                  {similar.map((sim) => (
                    <button
                      key={sim.id}
                      type="button"
                      className="similar-card"
                      onClick={() => onSelectBookmark(sim)}
                    >
                      <SimilarThumb item={sim} />
                      <span className="similar-card-title">{sim.title || sim.url}</span>
                      <span className="similar-card-domain">
                        {sim.site_name || domainFromURL(sim.url)}
                      </span>
                    </button>
                  ))}
                </div>
              ) : (
                !loadingSimilar && (
                  <p className="inspector-similar-empty">
                    No similar items found yet for these keywords.
                  </p>
                )
              )}
            </div>
          </div>

          <div className="inspector-footer">
            <div className="drawer-footer-left">
              <button
                type="button"
                className="btn btn-secondary btn-compact"
                onClick={handleArchiveClick}
                disabled={saving}
              >
                {active.archived ? (
                  <>
                    <ArchiveRestore size={14} aria-hidden="true" />
                    Restore
                  </>
                ) : (
                  <>
                    <Archive size={14} aria-hidden="true" />
                    Archive
                  </>
                )}
              </button>

              {confirmDelete ? (
                <button type="button" className="btn btn-danger btn-compact" onClick={executeDelete}>
                  Confirm
                </button>
              ) : (
                <button
                  type="button"
                  className={`btn btn-compact hold-delete-btn ${holding ? "is-holding" : ""}`}
                  onPointerDown={startHoldDelete}
                  onPointerUp={cancelHoldDelete}
                  onPointerLeave={cancelHoldDelete}
                  onPointerCancel={cancelHoldDelete}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      setConfirmDelete(true);
                    }
                  }}
                  title="Hold to delete (or press Enter to confirm)"
                >
                  <Trash2 size={14} aria-hidden="true" />
                  <span>Delete</span>
                  <span className="hold-delete-fill" aria-hidden="true">
                    <Trash2 size={14} />
                    Deleting...
                  </span>
                </button>
              )}
            </div>

            <button
              type="button"
              className="btn btn-primary btn-compact"
              onClick={handleSave}
              disabled={saving || !isDirty}
            >
              {saving ? "Saving..." : isDirty ? "Save changes" : "Saved"}
            </button>
          </div>
        </>
      )}
    </aside>
  );

  if (useModalDrawer) {
    return (
      <div
        className="drawer-overlay"
        data-state="open"
        onClick={(e) => {
          if (e.target === e.currentTarget) onClose();
        }}
      >
        {panelContent}
      </div>
    );
  }

  return panelContent;
}
