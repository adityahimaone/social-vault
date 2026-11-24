import { useState, type FormEvent } from "react";
import { FolderKanban, Plus, Trash2 } from "lucide-react";
import { api, ApiError } from "../api";
import type { List } from "../types";

interface ListsViewProps {
  lists: List[];
  onRefresh: () => Promise<void>;
  onFilterByList: (listName: string) => void;
  onNotify: (msg: string, tone: "success" | "error") => void;
}

export function ListsView({ lists, onRefresh, onFilterByList, onNotify }: ListsViewProps) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [busy, setBusy] = useState(false);

  async function handleCreate(e: FormEvent) {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;
    setBusy(true);
    try {
      await api.createList(trimmed, description.trim());
      setName("");
      setDescription("");
      await onRefresh();
      onNotify(`Created list "${trimmed}"`, "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to create list", "error");
    } finally {
      setBusy(false);
    }
  }

  async function handleDelete(list: List) {
    try {
      await api.deleteList(list.id);
      await onRefresh();
      onNotify(`Deleted list "${list.name}"`, "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to delete list", "error");
    }
  }

  return (
    <section className="content-panel" aria-labelledby="lists-heading">
      <div className="panel-header">
        <div>
          <h1 id="lists-heading" className="panel-title">
            Lists
          </h1>
          <p className="panel-subtitle">
            Group related bookmarks into curated collections.
          </p>
        </div>
        <span className="panel-count-pill tabular-nums">{lists.length} lists</span>
      </div>

      <form onSubmit={handleCreate} className="inline-form">
        <label className="sr-only" htmlFor="new-list-name">
          List name
        </label>
        <input
          id="new-list-name"
          type="text"
          className="input"
          placeholder="List name (e.g. X Threads, Inspiration)"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <label className="sr-only" htmlFor="new-list-desc">
          Description
        </label>
        <input
          id="new-list-desc"
          type="text"
          className="input"
          placeholder="Short description (optional)"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
        <button type="submit" className="btn btn-primary" disabled={busy || !name.trim()}>
          <Plus size={16} aria-hidden="true" />
          Create list
        </button>
      </form>

      {lists.length === 0 ? (
        <p className="panel-empty">
          No lists yet. Create one above or import folders from a Netscape HTML export.
        </p>
      ) : (
        <div className="grid-cards">
          {lists.map((list) => (
            <div key={list.id} className="entity-card">
              <button
                type="button"
                className="entity-card-btn"
                onClick={() => onFilterByList(list.name)}
              >
                <span className="entity-card-title">
                  <FolderKanban size={15} aria-hidden="true" className="entity-card-icon" />
                  <span className="entity-card-name">{list.name}</span>
                </span>
                {list.description && (
                  <span className="entity-card-desc">{list.description}</span>
                )}
                <span className="entity-card-meta tabular-nums">
                  {list.count ?? 0} bookmarks
                </span>
              </button>
              <button
                type="button"
                className="btn btn-ghost btn-icon btn-icon-sm"
                onClick={() => handleDelete(list)}
                aria-label={`Delete list ${list.name}`}
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
