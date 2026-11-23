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
    <section className="panel" aria-labelledby="lists-heading">
      <div>
        <h1 id="lists-heading" style={{ margin: 0, fontSize: 19, fontWeight: 700 }}>
          Lists
        </h1>
        <p style={{ margin: "4px 0 0", fontSize: 14, color: "var(--fg-muted)" }}>
          Group related bookmarks into curated collections.
        </p>
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
        <p style={{ margin: 0, fontSize: 14, color: "var(--fg-muted)" }}>
          No lists yet. Create one above or import folders from a Netscape HTML export.
        </p>
      ) : (
        <div className="grid-cards">
          {lists.map((list) => (
            <div key={list.id} className="entity-card">
              <button
                type="button"
                onClick={() => onFilterByList(list.name)}
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
                  <FolderKanban size={15} aria-hidden="true" style={{ color: "var(--accent-text)" }} />
                  <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    {list.name}
                  </span>
                </span>
                {list.description && (
                  <span style={{ fontSize: 13, color: "var(--fg-secondary)" }}>{list.description}</span>
                )}
                <span className="tabular-nums" style={{ fontSize: 12, color: "var(--fg-muted)" }}>
                  {list.count ?? 0} bookmarks
                </span>
              </button>
              <button
                type="button"
                className="btn btn-ghost btn-icon"
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
