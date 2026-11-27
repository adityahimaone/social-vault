import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import {
  ArrowLeft,
  Check,
  Edit3,
  ExternalLink,
  FolderKanban,
  Globe,
  Hash,
  Plus,
  Search,
  Sparkles,
  Trash2,
  X,
} from "lucide-react";
import { api, ApiError } from "../api";
import type { ArchivedFilter, Bookmark, List, SmartListRule, SortMode, Tag } from "../types";
import { domainFromURL, formatDate } from "../utils/bookmarks";
import { useFocusTrap } from "./glass/useFocusTrap";

interface ListDetailViewProps {
  list: List;
  allTags: Tag[];
  onBack: () => void;
  onRefreshMetadata: () => Promise<void>;
  onSelectBookmark: (bookmark: Bookmark) => void;
  onDeletedList: () => void;
  onNotify: (msg: string, tone: "success" | "error" | "info") => void;
}

export function formatRuleSummary(rule?: SmartListRule): string[] {
  if (!rule) return [];
  const chips: string[] = [];
  if (rule.tags && rule.tags.length > 0) {
    const joiner = rule.tag_mode === "all" ? " + " : " / ";
    chips.push(`Tags (${rule.tag_mode === "all" ? "all" : "any"}): ${rule.tags.map((t) => `#${t}`).join(joiner)}`);
  }
  if (rule.untagged) {
    chips.push("Untagged only");
  }
  if (rule.domain) {
    chips.push(`Domain: ${rule.domain}`);
  }
  if (rule.query) {
    chips.push(`Search: “${rule.query}”`);
  }
  if (rule.archived && rule.archived !== "all") {
    chips.push(rule.archived === "true" ? "Archived only" : "Active only");
  }
  return chips;
}

export function ListDetailView({
  list,
  allTags,
  onBack,
  onRefreshMetadata,
  onSelectBookmark,
  onDeletedList,
  onNotify,
}: ListDetailViewProps) {
  const [items, setItems] = useState<Bookmark[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(true);
  const [loadingMore, setLoadingMore] = useState(false);

  const [searchInput, setSearchInput] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [tagFilter, setTagFilter] = useState("");
  const [archivedFilter, setArchivedFilter] = useState<ArchivedFilter>("all");
  const [sortMode, setSortMode] = useState<SortMode>("newest");

  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState(list.name);
  const [editDesc, setEditDesc] = useState(list.description);
  const [editRuleTags, setEditRuleTags] = useState<string[]>(list.rule?.tags ?? []);
  const [editRuleTagMode, setEditRuleTagMode] = useState<"any" | "all">(list.rule?.tag_mode ?? "any");
  const [editRuleDomain, setEditRuleDomain] = useState(list.rule?.domain ?? "");
  const [editRuleQuery, setEditRuleQuery] = useState(list.rule?.query ?? "");
  const [editRuleUntagged, setEditRuleUntagged] = useState(Boolean(list.rule?.untagged));
  const [savingEdit, setSavingEdit] = useState(false);
  const [confirmDelete, setConfirmDelete] = useState(false);

  const [addModalOpen, setAddModalOpen] = useState(false);
  const [pickerQuery, setPickerQuery] = useState("");
  const [debouncedPickerQuery, setDebouncedPickerQuery] = useState("");
  const [pickerItems, setPickerItems] = useState<Bookmark[]>([]);
  const [pickerLoading, setPickerLoading] = useState(false);
  const [selectedAddIDs, setSelectedAddIDs] = useState<number[]>([]);
  const [addingBookmarks, setAddingBookmarks] = useState(false);
  const addModalRef = useRef<HTMLDivElement | null>(null);
  useFocusTrap(addModalRef, addModalOpen);

  useEffect(() => {
    setEditName(list.name);
    setEditDesc(list.description);
    setEditRuleTags(list.rule?.tags ?? []);
    setEditRuleTagMode(list.rule?.tag_mode ?? "any");
    setEditRuleDomain(list.rule?.domain ?? "");
    setEditRuleQuery(list.rule?.query ?? "");
    setEditRuleUntagged(Boolean(list.rule?.untagged));
    setEditing(false);
    setConfirmDelete(false);
  }, [list]);

  useEffect(() => {
    const t = window.setTimeout(() => setDebouncedQuery(searchInput.trim()), 220);
    return () => window.clearTimeout(t);
  }, [searchInput]);

  useEffect(() => {
    const t = window.setTimeout(() => setDebouncedPickerQuery(pickerQuery.trim()), 200);
    return () => window.clearTimeout(t);
  }, [pickerQuery]);

  const fetchListBookmarks = useCallback(
    async (signal?: AbortSignal) => {
      setLoading(true);
      try {
        const res = await api.listBookmarks(
          {
            list: String(list.id),
            q: debouncedQuery,
            tag: tagFilter,
            archived: archivedFilter,
            limit: 200,
            offset: 0,
          },
          signal
        );
        setItems(res.items);
        setTotal(res.total);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        onNotify(err instanceof ApiError ? err.message : "Failed to load list bookmarks", "error");
      } finally {
        setLoading(false);
      }
    },
    [list.id, debouncedQuery, tagFilter, archivedFilter, onNotify]
  );

  useEffect(() => {
    const ctrl = new AbortController();
    void fetchListBookmarks(ctrl.signal);
    return () => ctrl.abort();
  }, [fetchListBookmarks]);

  useEffect(() => {
    if (!addModalOpen) return;
    const ctrl = new AbortController();
    setPickerLoading(true);
    api
      .listBookmarks({ q: debouncedPickerQuery, limit: 60 }, ctrl.signal)
      .then((res) => {
        if (!ctrl.signal.aborted) {
          setPickerItems(res.items);
        }
      })
      .catch((err) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
      })
      .finally(() => {
        if (!ctrl.signal.aborted) setPickerLoading(false);
      });
    return () => ctrl.abort();
  }, [addModalOpen, debouncedPickerQuery]);

  async function handleLoadMore() {
    if (loadingMore || items.length >= total) return;
    setLoadingMore(true);
    try {
      const res = await api.listBookmarks({
        list: String(list.id),
        q: debouncedQuery,
        tag: tagFilter,
        archived: archivedFilter,
        limit: 200,
        offset: items.length,
      });
      setItems((prev) => {
        const seen = new Set(prev.map((b) => b.id));
        return [...prev, ...res.items.filter((b) => !seen.has(b.id))];
      });
      setTotal(res.total);
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to load more bookmarks", "error");
    } finally {
      setLoadingMore(false);
    }
  }

  const sortedItems = useMemo(() => {
    if (sortMode === "newest") {
      return [...items].sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at) || b.id - a.id);
    }
    if (sortMode === "oldest") {
      return [...items].sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at) || a.id - b.id);
    }
    return [...items].sort((a, b) =>
      (a.title || a.url).localeCompare(b.title || b.url, undefined, { sensitivity: "base" })
    );
  }, [items, sortMode]);

  async function handleSaveEdit(e: FormEvent) {
    e.preventDefault();
    const trimmed = editName.trim();
    if (!trimmed) return;
    setSavingEdit(true);
    try {
      const payload: Parameters<typeof api.updateList>[1] = {
        name: trimmed,
        description: editDesc.trim(),
      };
      if (list.is_smart) {
        payload.rule = {
          tags: editRuleTags,
          tag_mode: editRuleTagMode,
          domain: editRuleDomain.trim(),
          query: editRuleQuery.trim(),
          untagged: editRuleUntagged,
        };
      }
      await api.updateList(list.id, payload);
      await onRefreshMetadata();
      await fetchListBookmarks();
      setEditing(false);
      onNotify(`Updated list "${trimmed}"`, "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to update list", "error");
    } finally {
      setSavingEdit(false);
    }
  }

  async function handleDeleteList() {
    try {
      await api.deleteList(list.id);
      await onRefreshMetadata();
      onNotify(`Deleted list "${list.name}"`, "success");
      onDeletedList();
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to delete list", "error");
    }
  }

  async function handleRemoveBookmark(bookmark: Bookmark) {
    try {
      await api.removeBookmarkFromList(list.id, bookmark.id);
      setItems((prev) => prev.filter((b) => b.id !== bookmark.id));
      setTotal((n) => Math.max(0, n - 1));
      await onRefreshMetadata();
      onNotify(`Removed from "${list.name}"`, "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to remove bookmark", "error");
    }
  }

  async function handleConfirmAddBookmarks() {
    if (selectedAddIDs.length === 0 || addingBookmarks) return;
    setAddingBookmarks(true);
    try {
      const res = await api.addBookmarksToList(list.id, selectedAddIDs);
      setAddModalOpen(false);
      setSelectedAddIDs([]);
      setPickerQuery("");
      await Promise.all([onRefreshMetadata(), fetchListBookmarks()]);
      onNotify(`Added ${res.added} bookmark${res.added === 1 ? "" : "s"} to "${list.name}"`, "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Failed to add bookmarks", "error");
    } finally {
      setAddingBookmarks(false);
    }
  }

  const existingIDSet = useMemo(() => new Set(items.map((b) => b.id)), [items]);
  const ruleChips = formatRuleSummary(list.rule);

  return (
    <section className="content-panel" aria-labelledby="list-detail-heading">
      <div className="panel-header">
        <div className="list-detail-heading-group">
          <button type="button" className="btn btn-secondary btn-compact" onClick={onBack}>
            <ArrowLeft size={14} aria-hidden="true" />
            All lists
          </button>
          <div>
            <div className="list-detail-title-row">
              {list.is_smart ? (
                <Sparkles size={18} className="entity-card-icon" aria-hidden="true" />
              ) : (
                <FolderKanban size={18} className="entity-card-icon" aria-hidden="true" />
              )}
              <h1 id="list-detail-heading" className="panel-title">
                {list.name}
              </h1>
              {list.is_smart && <span className="smart-badge">Smart list</span>}
            </div>
            {list.description && <p className="panel-subtitle">{list.description}</p>}
            {ruleChips.length > 0 && (
              <div className="pill-row list-rule-chips">
                {ruleChips.map((chip) => (
                  <span key={chip} className="status-badge">
                    {chip}
                  </span>
                ))}
              </div>
            )}
          </div>
        </div>

        <div className="panel-header-actions">
          <span className="panel-count-pill tabular-nums">{total} bookmarks</span>
          {!list.is_smart && (
            <button
              type="button"
              className="btn btn-primary btn-compact"
              onClick={() => {
                setSelectedAddIDs([]);
                setAddModalOpen(true);
              }}
            >
              <Plus size={14} aria-hidden="true" />
              Add bookmarks
            </button>
          )}
          <button
            type="button"
            className="btn btn-secondary btn-compact"
            onClick={() => setEditing((v) => !v)}
          >
            <Edit3 size={14} aria-hidden="true" />
            Edit
          </button>
          {confirmDelete ? (
            <button type="button" className="btn btn-danger btn-compact" onClick={handleDeleteList}>
              Confirm delete
            </button>
          ) : (
            <button
              type="button"
              className="btn btn-ghost btn-icon btn-icon-sm"
              onClick={() => setConfirmDelete(true)}
              aria-label={`Delete list ${list.name}`}
              title="Delete list"
            >
              <Trash2 size={15} aria-hidden="true" />
            </button>
          )}
        </div>
      </div>

      {editing && (
        <form onSubmit={handleSaveEdit} className="autotag-card list-edit-card">
          <div className="inline-form">
            <input
              type="text"
              className="input"
              value={editName}
              onChange={(e) => setEditName(e.target.value)}
              placeholder="List name"
              required
            />
            <input
              type="text"
              className="input"
              value={editDesc}
              onChange={(e) => setEditDesc(e.target.value)}
              placeholder="Description (optional)"
            />
          </div>

          {list.is_smart && (
            <div className="smart-rule-editor">
              <div className="smart-rule-row">
                <div className="field">
                  <label className="field-label">Tags</label>
                  <div className="pill-row">
                    {allTags.map((t) => {
                      const active = editRuleTags.some((x) => x.toLowerCase() === t.name.toLowerCase());
                      return (
                        <button
                          key={t.id}
                          type="button"
                          className={active ? "list-pill" : "tag-pill"}
                          aria-pressed={active}
                          onClick={() =>
                            setEditRuleTags((prev) =>
                              active
                                ? prev.filter((x) => x.toLowerCase() !== t.name.toLowerCase())
                                : [...prev, t.name]
                            )
                          }
                        >
                          <Hash size={11} aria-hidden="true" />
                          {t.name}
                        </button>
                      );
                    })}
                  </div>
                </div>
              </div>

              <div className="inline-form">
                <select
                  className="select"
                  value={editRuleTagMode}
                  onChange={(e) => setEditRuleTagMode(e.target.value as "any" | "all")}
                  aria-label="Tag match mode"
                >
                  <option value="any">Match ANY selected tag</option>
                  <option value="all">Match ALL selected tags</option>
                </select>
                <input
                  type="text"
                  className="input"
                  value={editRuleDomain}
                  onChange={(e) => setEditRuleDomain(e.target.value)}
                  placeholder="Domain (e.g. x.com, threads.com)"
                />
                <input
                  type="text"
                  className="input"
                  value={editRuleQuery}
                  onChange={(e) => setEditRuleQuery(e.target.value)}
                  placeholder="Search keyword"
                />
              </div>

              <label className="smart-check-row">
                <input
                  type="checkbox"
                  checked={editRuleUntagged}
                  onChange={(e) => setEditRuleUntagged(e.target.checked)}
                />
                <span>Only untagged bookmarks</span>
              </label>
            </div>
          )}

          <div className="modal-actions">
            <button type="button" className="btn btn-secondary btn-compact" onClick={() => setEditing(false)}>
              Cancel
            </button>
            <button type="submit" className="btn btn-primary btn-compact" disabled={savingEdit || !editName.trim()}>
              {savingEdit ? "Saving..." : "Save list"}
            </button>
          </div>
        </form>
      )}

      <div className="list-filter-bar">
        <div className="search-wrap list-detail-search">
          <Search size={14} className="search-icon" aria-hidden="true" />
          <input
            type="search"
            className="input search-input"
            placeholder="Search inside this list..."
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
          />
          {searchInput && (
            <button
              type="button"
              className="search-clear-btn"
              onClick={() => setSearchInput("")}
              aria-label="Clear search"
            >
              <X size={13} aria-hidden="true" />
            </button>
          )}
        </div>

        <select
          className="select list-filter-select"
          value={tagFilter}
          onChange={(e) => setTagFilter(e.target.value)}
          aria-label="Filter by tag"
        >
          <option value="">All tags</option>
          {allTags.map((t) => (
            <option key={t.id} value={t.name}>
              #{t.name}
            </option>
          ))}
        </select>

        <select
          className="select list-filter-select"
          value={archivedFilter}
          onChange={(e) => setArchivedFilter(e.target.value as ArchivedFilter)}
          aria-label="Filter archive status"
        >
          <option value="all">All status</option>
          <option value="false">Active only</option>
          <option value="true">Archived only</option>
        </select>

        <select
          className="select list-filter-select"
          value={sortMode}
          onChange={(e) => setSortMode(e.target.value as SortMode)}
          aria-label="Sort bookmarks"
        >
          <option value="newest">Newest first</option>
          <option value="oldest">Oldest first</option>
          <option value="title">Title (A–Z)</option>
        </select>
      </div>

      {loading ? (
        <p className="panel-empty">Loading bookmarks...</p>
      ) : sortedItems.length === 0 ? (
        <div className="list-detail-empty">
          <p className="panel-empty">
            {debouncedQuery || tagFilter || archivedFilter !== "all"
              ? "No bookmarks match the current filter inside this list."
              : list.is_smart
                ? "No bookmarks currently match this smart list's rule."
                : "This list has no bookmarks yet."}
          </p>
          {!list.is_smart && !debouncedQuery && !tagFilter && (
            <button
              type="button"
              className="btn btn-primary btn-compact"
              onClick={() => setAddModalOpen(true)}
            >
              <Plus size={14} aria-hidden="true" />
              Add bookmarks to list
            </button>
          )}
        </div>
      ) : (
        <div className="bookmark-list">
          {sortedItems.map((item) => {
            const domain = item.site_name || domainFromURL(item.url);
            return (
              <article key={item.id} className="list-row-card">
                <button
                  type="button"
                  className="list-row-trigger"
                  onClick={() => onSelectBookmark(item)}
                >
                  <div className="list-row-content">
                    <div className="bookmark-meta-row">
                      <div className="bookmark-source">
                        <span className="favicon-box" aria-hidden="true">
                          <Globe size={12} />
                        </span>
                        <span className="bookmark-site">{domain}</span>
                      </div>
                      <time className="bookmark-date tabular-nums" dateTime={item.created_at}>
                        {formatDate(item.created_at)}
                      </time>
                    </div>
                    <span className="list-row-title">{item.title || item.url}</span>
                    {item.description && <p className="bookmark-desc">{item.description}</p>}
                  </div>
                </button>

                <div className="list-row-trailing">
                  <div className="pill-row">
                    {item.tags.slice(0, 3).map((tag) => (
                      <span key={tag.id} className="tag-pill">
                        <Hash size={11} aria-hidden="true" />
                        {tag.name}
                      </span>
                    ))}
                  </div>
                  <div className="bookmark-actions">
                    <a
                      href={item.url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="btn btn-ghost btn-icon btn-icon-sm"
                      aria-label={`Open ${item.title || item.url}`}
                      title="Open URL"
                    >
                      <ExternalLink size={14} aria-hidden="true" />
                    </a>
                    {!list.is_smart && (
                      <button
                        type="button"
                        className="btn btn-ghost btn-icon btn-icon-sm"
                        onClick={() => handleRemoveBookmark(item)}
                        aria-label={`Remove ${item.title || item.url} from list`}
                        title="Remove from list"
                      >
                        <X size={14} aria-hidden="true" />
                      </button>
                    )}
                  </div>
                </div>
              </article>
            );
          })}
        </div>
      )}

      {items.length < total && (
        <div className="load-more-row">
          <button
            type="button"
            className="btn btn-secondary"
            onClick={handleLoadMore}
            disabled={loadingMore}
          >
            {loadingMore ? "Loading more..." : "Load more"}
          </button>
        </div>
      )}

      {addModalOpen && (
        <div
          className="overlay"
          data-state="open"
          onClick={(e) => {
            if (e.target === e.currentTarget) setAddModalOpen(false);
          }}
        >
          <div
            ref={addModalRef}
            className="modal-panel picker-modal"
            role="dialog"
            aria-modal="true"
            aria-labelledby="add-to-list-heading"
          >
            <div className="modal-header">
              <div className="modal-title-group">
                <span className="modal-icon-well" aria-hidden="true">
                  <FolderKanban size={16} />
                </span>
                <h2 id="add-to-list-heading" className="modal-title">
                  Add bookmarks to {list.name}
                </h2>
              </div>
              <button
                type="button"
                className="btn btn-ghost btn-icon"
                onClick={() => setAddModalOpen(false)}
                aria-label="Close modal"
              >
                <X size={18} aria-hidden="true" />
              </button>
            </div>

            <div className="search-wrap">
              <Search size={14} className="search-icon" aria-hidden="true" />
              <input
                type="search"
                className="input search-input"
                placeholder="Search bookmarks by title, notes, or URL..."
                value={pickerQuery}
                onChange={(e) => setPickerQuery(e.target.value)}
                autoFocus
              />
            </div>

            <div className="picker-list">
              {pickerLoading ? (
                <p className="panel-empty">Searching vault...</p>
              ) : pickerItems.length === 0 ? (
                <p className="panel-empty">No matching bookmarks found.</p>
              ) : (
                pickerItems.map((bm) => {
                  const alreadyInList =
                    existingIDSet.has(bm.id) || bm.lists.some((l) => l.id === list.id);
                  const checked = selectedAddIDs.includes(bm.id);
                  return (
                    <label
                      key={bm.id}
                      className={`picker-item ${alreadyInList ? "is-disabled" : checked ? "is-selected" : ""}`}
                    >
                      <input
                        type="checkbox"
                        disabled={alreadyInList}
                        checked={alreadyInList || checked}
                        onChange={() => {
                          if (alreadyInList) return;
                          setSelectedAddIDs((prev) =>
                            checked ? prev.filter((id) => id !== bm.id) : [...prev, bm.id]
                          );
                        }}
                      />
                      <span className="picker-item-copy">
                        <span className="picker-item-title">{bm.title || bm.url}</span>
                        <span className="picker-item-meta">
                          {bm.site_name || domainFromURL(bm.url)}
                          {alreadyInList ? " · Already in list" : ""}
                        </span>
                      </span>
                      {alreadyInList && <Check size={14} aria-hidden="true" />}
                    </label>
                  );
                })
              )}
            </div>

            <div className="modal-actions">
              <button
                type="button"
                className="btn btn-secondary"
                onClick={() => setAddModalOpen(false)}
              >
                Cancel
              </button>
              <button
                type="button"
                className="btn btn-primary"
                disabled={selectedAddIDs.length === 0 || addingBookmarks}
                onClick={handleConfirmAddBookmarks}
              >
                {addingBookmarks
                  ? "Adding..."
                  : `Add ${selectedAddIDs.length || ""} bookmark${selectedAddIDs.length === 1 ? "" : "s"}`}
              </button>
            </div>
          </div>
        </div>
      )}
    </section>
  );
}
