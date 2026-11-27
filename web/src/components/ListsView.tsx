import { useState, type FormEvent } from "react";
import { Edit3, FolderKanban, Hash, Plus, Sparkles, Trash2 } from "lucide-react";
import { api, ApiError } from "../api";
import type { Bookmark, List, Tag } from "../types";
import { ListDetailView, formatRuleSummary } from "./ListDetailView";

interface ListsViewProps {
  lists: List[];
  allTags: Tag[];
  activeListId: number | null;
  onSelectListId: (id: number | null) => void;
  onRefresh: () => Promise<void>;
  onSelectBookmark: (bookmark: Bookmark) => void;
  onNotify: (msg: string, tone: "success" | "error" | "info") => void;
}

export function ListsView({
  lists,
  allTags,
  activeListId,
  onSelectListId,
  onRefresh,
  onSelectBookmark,
  onNotify,
}: ListsViewProps) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [isSmart, setIsSmart] = useState(false);
  const [ruleTags, setRuleTags] = useState<string[]>([]);
  const [ruleTagMode, setRuleTagMode] = useState<"any" | "all">("any");
  const [ruleDomain, setRuleDomain] = useState("");
  const [ruleQuery, setRuleQuery] = useState("");
  const [ruleUntagged, setRuleUntagged] = useState(false);
  const [busy, setBusy] = useState(false);

  const [renamingId, setRenamingId] = useState<number | null>(null);
  const [renameValue, setRenameValue] = useState("");
  const [renameDescValue, setRenameDescValue] = useState("");

  const activeList = activeListId != null ? lists.find((l) => l.id === activeListId) ?? null : null;

  if (activeList) {
    return (
      <ListDetailView
        list={activeList}
        allTags={allTags}
        onBack={() => onSelectListId(null)}
        onRefreshMetadata={onRefresh}
        onSelectBookmark={onSelectBookmark}
        onDeletedList={() => onSelectListId(null)}
        onNotify={onNotify}
      />
    );
  }

  async function handleCreate(e: FormEvent) {
    e.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;
    setBusy(true);
    try {
      const created = await api.createList({
        name: trimmed,
        description: description.trim(),
        is_smart: isSmart,
        rule: isSmart
          ? {
              tags: ruleTags,
              tag_mode: ruleTagMode,
              domain: ruleDomain.trim(),
              query: ruleQuery.trim(),
              untagged: ruleUntagged,
            }
          : undefined,
      });
      setName("");
      setDescription("");
      setIsSmart(false);
      setRuleTags([]);
      setRuleTagMode("any");
      setRuleDomain("");
      setRuleQuery("");
      setRuleUntagged(false);
      await onRefresh();
      onNotify(`Created ${created.is_smart ? "smart " : ""}list "${trimmed}"`, "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to create list", "error");
    } finally {
      setBusy(false);
    }
  }

  async function handleQuickRename(e: FormEvent, list: List) {
    e.preventDefault();
    const trimmed = renameValue.trim();
    if (!trimmed) return;
    try {
      await api.updateList(list.id, {
        name: trimmed,
        description: renameDescValue.trim(),
      });
      setRenamingId(null);
      await onRefresh();
      onNotify(`Updated list "${trimmed}"`, "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to rename list", "error");
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

  const hasValidSmartRule =
    ruleTags.length > 0 ||
    ruleDomain.trim().length > 0 ||
    ruleQuery.trim().length > 0 ||
    ruleUntagged;

  return (
    <section className="content-panel" aria-labelledby="lists-heading">
      <div className="panel-header">
        <div>
          <h1 id="lists-heading" className="panel-title">
            Lists
          </h1>
          <p className="panel-subtitle">
            Group bookmarks into curated manual lists or rule-based smart collections.
          </p>
        </div>
        <span className="panel-count-pill tabular-nums">{lists.length} lists</span>
      </div>

      <form onSubmit={handleCreate} className="autotag-card">
        <div className="autotag-header">
          <div>
            <h2 className="autotag-title">Create a list</h2>
            <p className="autotag-subtitle">
              Manual lists hold hand-picked bookmarks; smart lists auto-populate from rules.
            </p>
          </div>
          <div className="popover-chip-row" role="radiogroup" aria-label="List type">
            <button
              type="button"
              role="radio"
              aria-checked={!isSmart}
              className={`popover-chip ${!isSmart ? "is-selected" : ""}`}
              onClick={() => setIsSmart(false)}
            >
              <FolderKanban size={12} aria-hidden="true" />
              Manual list
            </button>
            <button
              type="button"
              role="radio"
              aria-checked={isSmart}
              className={`popover-chip ${isSmart ? "is-selected" : ""}`}
              onClick={() => setIsSmart(true)}
            >
              <Sparkles size={12} aria-hidden="true" />
              Smart list
            </button>
          </div>
        </div>

        <div className="inline-form">
          <label className="sr-only" htmlFor="new-list-name">
            List name
          </label>
          <input
            id="new-list-name"
            type="text"
            className="input"
            placeholder={isSmart ? "Smart list name (e.g. AI Tools, X Reads)" : "List name (e.g. Reading Queue, Inspiration)"}
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
          <button
            type="submit"
            className="btn btn-primary"
            disabled={busy || !name.trim() || (isSmart && !hasValidSmartRule)}
          >
            <Plus size={16} aria-hidden="true" />
            {isSmart ? "Create smart list" : "Create list"}
          </button>
        </div>

        {isSmart && (
          <div className="smart-rule-editor">
            {allTags.length > 0 && (
              <div className="field">
                <span className="field-label">Include bookmarks with tags</span>
                <div className="pill-row">
                  {allTags.map((tag) => {
                    const active = ruleTags.some((t) => t.toLowerCase() === tag.name.toLowerCase());
                    return (
                      <button
                        key={tag.id}
                        type="button"
                        className={active ? "list-pill" : "tag-pill"}
                        aria-pressed={active}
                        onClick={() =>
                          setRuleTags((prev) =>
                            active
                              ? prev.filter((t) => t.toLowerCase() !== tag.name.toLowerCase())
                              : [...prev, tag.name]
                          )
                        }
                      >
                        <Hash size={11} aria-hidden="true" />
                        {tag.name}
                      </button>
                    );
                  })}
                </div>
              </div>
            )}

            <div className="inline-form">
              <select
                className="select"
                value={ruleTagMode}
                onChange={(e) => setRuleTagMode(e.target.value as "any" | "all")}
                aria-label="Tag match mode"
              >
                <option value="any">Match ANY selected tag</option>
                <option value="all">Match ALL selected tags</option>
              </select>

              <input
                type="text"
                className="input"
                placeholder="Domain filter (e.g. x.com, threads.com)"
                value={ruleDomain}
                onChange={(e) => setRuleDomain(e.target.value)}
              />

              <input
                type="text"
                className="input"
                placeholder="Keyword filter (optional)"
                value={ruleQuery}
                onChange={(e) => setRuleQuery(e.target.value)}
              />
            </div>

            <div className="smart-rule-footer">
              <div className="popover-chip-row">
                <button
                  type="button"
                  className={`popover-chip ${ruleDomain === "x.com" ? "is-selected" : ""}`}
                  onClick={() => setRuleDomain((d) => (d === "x.com" ? "" : "x.com"))}
                >
                  x.com
                </button>
                <button
                  type="button"
                  className={`popover-chip ${ruleDomain === "threads.com" ? "is-selected" : ""}`}
                  onClick={() => setRuleDomain((d) => (d === "threads.com" ? "" : "threads.com"))}
                >
                  threads.com
                </button>
              </div>
              <label className="smart-check-row">
                <input
                  type="checkbox"
                  checked={ruleUntagged}
                  onChange={(e) => setRuleUntagged(e.target.checked)}
                />
                <span>Only untagged bookmarks</span>
              </label>
            </div>
          </div>
        )}
      </form>

      {lists.length === 0 ? (
        <p className="panel-empty">
          No lists yet. Create a manual or smart list above to start organizing your 600+ bookmarks.
        </p>
      ) : (
        <div className="grid-cards">
          {lists.map((list) => {
            const ruleChips = formatRuleSummary(list.rule);
            const isRenaming = renamingId === list.id;
            return (
              <div key={list.id} className="entity-card entity-card-stack">
                {isRenaming ? (
                  <form onSubmit={(e) => handleQuickRename(e, list)} className="entity-inline-edit">
                    <input
                      type="text"
                      className="input"
                      value={renameValue}
                      onChange={(e) => setRenameValue(e.target.value)}
                      autoFocus
                      required
                    />
                    <input
                      type="text"
                      className="input"
                      value={renameDescValue}
                      onChange={(e) => setRenameDescValue(e.target.value)}
                      placeholder="Description"
                    />
                    <div className="entity-inline-actions">
                      <button type="button" className="btn btn-secondary btn-compact" onClick={() => setRenamingId(null)}>
                        Cancel
                      </button>
                      <button type="submit" className="btn btn-primary btn-compact" disabled={!renameValue.trim()}>
                        Save
                      </button>
                    </div>
                  </form>
                ) : (
                  <div className="entity-card-top">
                    <button
                      type="button"
                      className="entity-card-btn"
                      onClick={() => onSelectListId(list.id)}
                    >
                      <span className="entity-card-title">
                        {list.is_smart ? (
                          <Sparkles size={15} aria-hidden="true" className="entity-card-icon" />
                        ) : (
                          <FolderKanban size={15} aria-hidden="true" className="entity-card-icon" />
                        )}
                        <span className="entity-card-name">{list.name}</span>
                        {list.is_smart && <span className="smart-badge">Smart</span>}
                      </span>
                      {list.description && (
                        <span className="entity-card-desc">{list.description}</span>
                      )}
                      <span className="entity-card-meta tabular-nums">
                        {list.count ?? 0} bookmarks
                      </span>
                    </button>
                    <div className="entity-card-actions">
                      <button
                        type="button"
                        className="btn btn-ghost btn-icon btn-icon-sm"
                        onClick={() => {
                          setRenamingId(list.id);
                          setRenameValue(list.name);
                          setRenameDescValue(list.description);
                        }}
                        aria-label={`Edit list ${list.name}`}
                        title="Rename list"
                      >
                        <Edit3 size={14} aria-hidden="true" />
                      </button>
                      <button
                        type="button"
                        className="btn btn-ghost btn-icon btn-icon-sm"
                        onClick={() => handleDelete(list)}
                        aria-label={`Delete list ${list.name}`}
                        title="Delete list"
                      >
                        <Trash2 size={14} aria-hidden="true" />
                      </button>
                    </div>
                  </div>
                )}

                {!isRenaming && ruleChips.length > 0 && (
                  <div className="pill-row">
                    {ruleChips.map((chip) => (
                      <span key={chip} className="status-badge">
                        {chip}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}
    </section>
  );
}
