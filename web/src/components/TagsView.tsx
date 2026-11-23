import { useState, type FormEvent } from "react";
import { Hash, Plus, Trash2 } from "lucide-react";
import { api, ApiError } from "../api";
import type { Tag } from "../types";

interface TagsViewProps {
  tags: Tag[];
  onRefresh: () => Promise<void>;
  onFilterByTag: (tagName: string) => void;
  onNotify: (msg: string, tone: "success" | "error") => void;
}

export function TagsView({ tags, onRefresh, onFilterByTag, onNotify }: TagsViewProps) {
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);

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
      onNotify(`Deleted tag #${tag.name}`, "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to delete tag", "error");
    }
  }

  return (
    <section className="panel" aria-labelledby="tags-heading">
      <div>
        <h1 id="tags-heading" style={{ margin: 0, fontSize: 19, fontWeight: 700 }}>
          Tags
        </h1>
        <p style={{ margin: "4px 0 0", fontSize: 14, color: "var(--fg-muted)" }}>
          Organize bookmarks with lightweight labels. Click any tag to filter your vault.
        </p>
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

      {tags.length === 0 ? (
        <p style={{ margin: 0, fontSize: 14, color: "var(--fg-muted)" }}>
          No tags yet. Tags added to bookmarks appear here automatically.
        </p>
      ) : (
        <div className="grid-cards">
          {tags.map((tag) => (
            <div key={tag.id} className="entity-card">
              <button
                type="button"
                onClick={() => onFilterByTag(tag.name)}
                style={{
                  background: "none",
                  border: "none",
                  padding: 0,
                  textAlign: "left",
                  display: "flex",
                  flexDirection: "column",
                  gap: 4,
                  minWidth: 0,
                  flex: 1,
                }}
              >
                <span style={{ display: "flex", alignItems: "center", gap: 6, fontWeight: 600, fontSize: 15 }}>
                  <Hash size={15} aria-hidden="true" style={{ color: "var(--accent-text)" }} />
                  <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    {tag.name}
                  </span>
                </span>
                <span className="tabular-nums" style={{ fontSize: 12, color: "var(--fg-muted)" }}>
                  {tag.count ?? 0} bookmarks
                </span>
              </button>
              <button
                type="button"
                className="btn btn-ghost btn-icon"
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
