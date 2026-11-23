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
    <section className="glass-panel glass-rim" aria-labelledby="tags-heading">
      <div className="panel-header">
        <div>
          <h1 id="tags-heading" className="panel-title">
            Tags
          </h1>
          <p className="panel-subtitle">
            Organize bookmarks with lightweight labels. Click any tag to filter your vault.
          </p>
        </div>
        <span className="panel-count-pill tabular-nums">{tags.length} tags</span>
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
        <p className="panel-empty">
          No tags yet. Tags added to bookmarks appear here automatically.
        </p>
      ) : (
        <div className="grid-cards">
          {tags.map((tag) => (
            <div key={tag.id} className="entity-card glass-rim">
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
