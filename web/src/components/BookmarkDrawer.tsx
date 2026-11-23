import { useEffect, useRef, useState } from "react";
import { Archive, ArchiveRestore, ExternalLink, Hash, Trash2, X } from "lucide-react";
import { api, ApiError } from "../api";
import type { Bookmark, List } from "../types";

interface BookmarkDrawerProps {
  bookmark: Bookmark | null;
  instantOpen: boolean;
  allLists: List[];
  onClose: (instant?: boolean) => void;
  onUpdated: (bookmark: Bookmark) => void;
  onDeleted: (id: number) => void;
  onNotify: (msg: string, tone: "success" | "error") => void;
}

export function BookmarkDrawer({
  bookmark,
  instantOpen,
  allLists,
  onClose,
  onUpdated,
  onDeleted,
  onNotify,
}: BookmarkDrawerProps) {
  const [animState, setAnimState] = useState<"entering" | "open" | "closing">("open");
  const [instant, setInstant] = useState(false);

  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [notes, setNotes] = useState("");
  const [tags, setTags] = useState<string[]>([]);
  const [tagDraft, setTagDraft] = useState("");
  const [selectedListIDs, setSelectedListIDs] = useState<number[]>([]);
  const [saving, setSaving] = useState(false);

  // Hold-to-delete + keyboard fallback confirm
  const [holding, setHolding] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);
  const holdTimerRef = useRef<number | null>(null);
  const titleInputRef = useRef<HTMLInputElement | null>(null);
  const openerRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!bookmark) return;
    openerRef.current = document.activeElement as HTMLElement | null;
    setTitle(bookmark.title);
    setDescription(bookmark.description);
    setNotes(bookmark.notes);
    setTags(bookmark.tags.map((t) => t.name));
    setSelectedListIDs(bookmark.lists.map((l) => l.id));
    setTagDraft("");
    setConfirmDelete(false);
    setHolding(false);
    setInstant(instantOpen);

    if (!instantOpen) {
      setAnimState("entering");
      const raf = requestAnimationFrame(() => {
        setAnimState("open");
      });
      titleInputRef.current?.focus();
      return () => cancelAnimationFrame(raf);
    } else {
      setAnimState("open");
      titleInputRef.current?.focus();
    }
  }, [bookmark, instantOpen]);

  useEffect(() => {
    if (!bookmark) return;
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        requestClose(true);
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  });

  if (!bookmark) return null;
  const activeBookmark = bookmark;

  function requestClose(viaKeyboard = false) {
    if (viaKeyboard) {
      setInstant(true);
      onClose(true);
      openerRef.current?.focus();
      return;
    }
    setInstant(false);
    setAnimState("closing");
    window.setTimeout(() => {
      onClose(false);
      openerRef.current?.focus();
    }, 210);
  }

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

  async function handleSave() {
    setSaving(true);
    try {
      const updated = await api.updateBookmark(activeBookmark.id, {
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

  async function handleToggleArchive() {
    setSaving(true);
    try {
      const updated = await api.updateBookmark(activeBookmark.id, {
        archived: !activeBookmark.archived,
      });
      onUpdated(updated);
      onNotify(updated.archived ? "Bookmark archived" : "Bookmark restored", "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to update archive state", "error");
    } finally {
      setSaving(false);
    }
  }

  async function executeDelete() {
    try {
      await api.deleteBookmark(activeBookmark.id);
      onDeleted(activeBookmark.id);
      onNotify("Bookmark deleted", "success");
      requestClose(true);
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

  return (
    <div
      className="drawer-overlay"
      data-state={animState}
      data-instant={instant ? "true" : "false"}
      onClick={(e) => {
        if (e.target === e.currentTarget) requestClose(false);
      }}
    >
      <div
        className="drawer-panel"
        role="dialog"
        aria-modal="true"
        aria-labelledby="drawer-heading"
      >
        <div className="drawer-header">
          <h2 id="drawer-heading" style={{ margin: 0, fontSize: 16, fontWeight: 600 }}>
            Bookmark Details
          </h2>
          <div style={{ display: "flex", alignItems: "center", gap: 6 }}>
            <a
              href={activeBookmark.url}
              target="_blank"
              rel="noopener noreferrer"
              className="btn btn-ghost btn-icon"
              aria-label="Open URL in new tab"
            >
              <ExternalLink size={16} aria-hidden="true" />
            </a>
            <button
              type="button"
              className="btn btn-ghost btn-icon"
              onClick={() => requestClose(false)}
              aria-label="Close drawer"
            >
              <X size={18} aria-hidden="true" />
            </button>
          </div>
        </div>

        <div className="drawer-body">
          <div style={{ fontSize: 12, color: "var(--fg-muted)", wordBreak: "break-all" }}>
            <a href={activeBookmark.url} target="_blank" rel="noopener noreferrer">
              {activeBookmark.url}
            </a>
          </div>

          <div className="field">
            <label className="field-label" htmlFor="edit-title">
              Title
            </label>
            <input
              ref={titleInputRef}
              id="edit-title"
              type="text"
              className="input"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
            />
          </div>

          <div className="field">
            <label className="field-label" htmlFor="edit-desc">
              Description
            </label>
            <textarea
              id="edit-desc"
              className="textarea"
              style={{ minHeight: 74 }}
              value={description}
              onChange={(e) => setDescription(e.target.value)}
            />
          </div>

          <div className="field">
            <label className="field-label" htmlFor="edit-notes">
              Personal Notes
            </label>
            <textarea
              id="edit-notes"
              className="textarea"
              placeholder="Why you saved this link, key quotes, takeaways..."
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
            />
          </div>

          <div className="field">
            <label className="field-label" htmlFor="edit-tag-input">
              Tags
            </label>
            <div style={{ display: "flex", gap: 8 }}>
              <input
                id="edit-tag-input"
                type="text"
                className="input"
                placeholder="Add a tag and press Enter"
                value={tagDraft}
                onChange={(e) => setTagDraft(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    addTagFromDraft();
                  }
                }}
              />
              <button type="button" className="btn btn-secondary" onClick={addTagFromDraft}>
                Add
              </button>
            </div>
            {tags.length > 0 && (
              <div className="pill-row" style={{ marginTop: 6 }}>
                {tags.map((tag) => (
                  <button
                    key={tag}
                    type="button"
                    className="tag-pill"
                    onClick={() => removeTag(tag)}
                    aria-label={`Remove tag ${tag}`}
                  >
                    <Hash size={12} aria-hidden="true" />
                    {tag}
                    <X size={12} aria-hidden="true" />
                  </button>
                ))}
              </div>
            )}
          </div>

          {allLists.length > 0 && (
            <div className="field">
              <span className="field-label">Lists</span>
              <div style={{ display: "flex", flexWrap: "wrap", gap: 8 }}>
                {allLists.map((list) => {
                  const checked = selectedListIDs.includes(list.id);
                  return (
                    <button
                      key={list.id}
                      type="button"
                      className={checked ? "list-pill" : "tag-pill"}
                      aria-pressed={checked}
                      onClick={() => toggleList(list.id)}
                    >
                      {list.name}
                    </button>
                  );
                })}
              </div>
            </div>
          )}
        </div>

        <div className="drawer-footer">
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <button
              type="button"
              className="btn btn-secondary"
              onClick={handleToggleArchive}
              disabled={saving}
            >
              {activeBookmark.archived ? (
                <>
                  <ArchiveRestore size={15} aria-hidden="true" />
                  Restore
                </>
              ) : (
                <>
                  <Archive size={15} aria-hidden="true" />
                  Archive
                </>
              )}
            </button>

            {confirmDelete ? (
              <button type="button" className="btn btn-danger" onClick={executeDelete}>
                Confirm Delete
              </button>
            ) : (
              <button
                type="button"
                className={`btn hold-delete-btn ${holding ? "is-holding" : ""}`}
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
                title="Hold to delete (or press Enter for keyboard confirm)"
              >
                <Trash2 size={15} aria-hidden="true" />
                <span>Hold to delete</span>
                <span className="hold-delete-fill" aria-hidden="true">
                  <Trash2 size={15} />
                  Deleting...
                </span>
              </button>
            )}
          </div>

          <button type="button" className="btn btn-primary" onClick={handleSave} disabled={saving}>
            {saving ? "Saving..." : "Save"}
          </button>
        </div>
      </div>
    </div>
  );
}
