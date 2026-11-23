import { useEffect, useRef, useState, type FormEvent } from "react";
import { X } from "lucide-react";
import { api, ApiError } from "../api";
import type { Bookmark, List } from "../types";

interface AddBookmarkModalProps {
  open: boolean;
  instantOpen: boolean;
  lists: List[];
  onClose: (instant?: boolean) => void;
  onCreated: (bookmark: Bookmark) => void;
  onNotify: (msg: string, tone: "success" | "error") => void;
}

export function AddBookmarkModal({
  open,
  instantOpen,
  lists,
  onClose,
  onCreated,
  onNotify,
}: AddBookmarkModalProps) {
  const [animState, setAnimState] = useState<"entering" | "open" | "closing">("open");
  const [instant, setInstant] = useState(false);

  const [url, setUrl] = useState("");
  const [tagsRaw, setTagsRaw] = useState("");
  const [selectedList, setSelectedList] = useState("");
  const [notes, setNotes] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const urlInputRef = useRef<HTMLInputElement | null>(null);
  const openerRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    if (!open) return;
    openerRef.current = document.activeElement as HTMLElement | null;
    setUrl("");
    setTagsRaw("");
    setSelectedList("");
    setNotes("");
    setError(null);
    setInstant(instantOpen);

    if (!instantOpen) {
      setAnimState("entering");
      const raf = requestAnimationFrame(() => setAnimState("open"));
      urlInputRef.current?.focus();
      return () => cancelAnimationFrame(raf);
    } else {
      setAnimState("open");
      urlInputRef.current?.focus();
    }
  }, [open, instantOpen]);

  useEffect(() => {
    if (!open) return;
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        requestClose(true);
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  });

  if (!open) return null;

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
    }, 160);
  }

  async function handleSubmit(e: FormEvent) {
    e.preventDefault();
    const trimmed = url.trim();
    if (!trimmed) {
      setError("Paste an http:// or https:// URL.");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      const tags = tagsRaw
        .split(",")
        .map((t) => t.trim())
        .filter(Boolean);
      const created = await api.createBookmark({
        url: trimmed,
        notes: notes.trim() || undefined,
        tags,
        lists: selectedList ? [selectedList] : undefined,
      });
      onCreated(created);
      onNotify("Bookmark saved", "success");
      requestClose(false);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to save bookmark");
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div
      className="overlay"
      data-state={animState}
      data-instant={instant ? "true" : "false"}
      onClick={(e) => {
        if (e.target === e.currentTarget) requestClose(false);
      }}
    >
      <div
        className="modal-panel"
        role="dialog"
        aria-modal="true"
        aria-labelledby="add-bookmark-heading"
      >
        <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between" }}>
          <h2 id="add-bookmark-heading" style={{ margin: 0, fontSize: 17, fontWeight: 600 }}>
            Save URL
          </h2>
          <button
            type="button"
            className="btn btn-ghost btn-icon"
            onClick={() => requestClose(false)}
            aria-label="Close modal"
          >
            <X size={18} aria-hidden="true" />
          </button>
        </div>

        <form onSubmit={handleSubmit} style={{ display: "flex", flexDirection: "column", gap: 14 }}>
          <div className="field">
            <label className="field-label" htmlFor="add-url">
              URL
            </label>
            <input
              ref={urlInputRef}
              id="add-url"
              type="url"
              className="input"
              placeholder="https://x.com/..."
              value={url}
              onChange={(e) => setUrl(e.target.value)}
              required
            />
            <span className="field-hint">
              Title, description, and favicon are fetched automatically when available.
            </span>
          </div>

          <div className="field">
            <label className="field-label" htmlFor="add-tags">
              Tags (comma-separated)
            </label>
            <input
              id="add-tags"
              type="text"
              className="input"
              placeholder="threads, golang, design"
              value={tagsRaw}
              onChange={(e) => setTagsRaw(e.target.value)}
            />
          </div>

          {lists.length > 0 && (
            <div className="field">
              <label className="field-label" htmlFor="add-list">
                List (optional)
              </label>
              <select
                id="add-list"
                className="select"
                value={selectedList}
                onChange={(e) => setSelectedList(e.target.value)}
              >
                <option value="">No list</option>
                {lists.map((l) => (
                  <option key={l.id} value={l.name}>
                    {l.name}
                  </option>
                ))}
              </select>
            </div>
          )}

          <div className="field">
            <label className="field-label" htmlFor="add-notes">
              Notes (optional)
            </label>
            <textarea
              id="add-notes"
              className="textarea"
              style={{ minHeight: 74 }}
              placeholder="Quick context for later search..."
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
            />
          </div>

          {error && (
            <div className="field-error" role="alert">
              {error}
            </div>
          )}

          <div style={{ display: "flex", justifyContent: "flex-end", gap: 10, marginTop: 4 }}>
            <button type="button" className="btn btn-secondary" onClick={() => requestClose(false)}>
              Cancel
            </button>
            <button type="submit" className="btn btn-primary" disabled={submitting}>
              {submitting ? "Fetching & saving..." : "Save bookmark"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
