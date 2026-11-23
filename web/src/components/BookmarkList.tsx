import { useState } from "react";
import { Archive, ArchiveRestore, Bookmark as BookmarkIcon, ExternalLink, FolderKanban, Globe, Hash, Plus, Upload } from "lucide-react";
import type { Bookmark } from "../types";

interface BookmarkListProps {
  items: Bookmark[];
  loading: boolean;
  viewMode: "grid" | "list";
  hasActiveFilters: boolean;
  onOpenDetail: (bookmark: Bookmark) => void;
  onToggleArchive: (bookmark: Bookmark) => void;
  onSelectTag: (tagName: string) => void;
  onSelectList: (listName: string) => void;
  onOpenAdd: () => void;
  onOpenImport?: () => void;
  onClearFilters: () => void;
}

function Favicon({ src }: { src: string }) {
  const [failed, setFailed] = useState(false);
  if (!src || failed) {
    return (
      <span className="favicon-box" aria-hidden="true">
        <Globe size={13} />
      </span>
    );
  }
  return (
    <span className="favicon-box" aria-hidden="true">
      <img src={src} alt="" loading="lazy" onError={() => setFailed(true)} />
    </span>
  );
}

function formatDate(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}

function domainFromURL(raw: string): string {
  try {
    return new URL(raw).hostname.replace(/^www\./, "");
  } catch {
    return raw;
  }
}

export function BookmarkList({
  items,
  loading,
  viewMode,
  hasActiveFilters,
  onOpenDetail,
  onToggleArchive,
  onSelectTag,
  onSelectList,
  onOpenAdd,
  onOpenImport,
  onClearFilters,
}: BookmarkListProps) {
  if (loading && items.length === 0) {
    return (
      <div className={viewMode === "grid" ? "bookmark-grid" : "bookmark-list"} aria-busy="true" aria-label="Loading bookmarks">
        {[0, 1, 2, 3].map((idx) => (
          <div key={idx} className="bookmark-skeleton glass-rim" aria-hidden="true">
            <div className="skeleton-line skeleton-line-short" />
            <div className="skeleton-line skeleton-line-title" />
            <div className="skeleton-line skeleton-line-body" />
          </div>
        ))}
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <div className="empty-state glass-rim">
        <span className="empty-state-icon" aria-hidden="true">
          <BookmarkIcon size={22} />
        </span>
        <h2 className="empty-state-title">
          {hasActiveFilters ? "No matching bookmarks" : "Your vault is empty"}
        </h2>
        <p className="empty-state-desc">
          {hasActiveFilters
            ? "Try a broader search term or clear your active tag, list, or archive filters."
            : "Paste any URL from X, Threads, or the web to fetch metadata automatically, or import an existing archive."}
        </p>
        <div className="empty-state-actions">
          {hasActiveFilters ? (
            <button type="button" className="btn btn-secondary" onClick={onClearFilters}>
              Clear filters
            </button>
          ) : (
            <>
              <button type="button" className="btn btn-primary" onClick={onOpenAdd}>
                <Plus size={16} aria-hidden="true" />
                Save first URL
              </button>
              {onOpenImport && (
                <button type="button" className="btn btn-glass" onClick={onOpenImport}>
                  <Upload size={15} aria-hidden="true" />
                  Import bookmarks
                </button>
              )}
            </>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className={viewMode === "grid" ? "bookmark-grid" : "bookmark-list"} role="list">
      {items.map((item) => (
        <article
          key={item.id}
          className={`bookmark-card glass-rim ${item.archived ? "is-archived" : ""}`}
          role="listitem"
        >
          <div className="bookmark-main">
            <div className="bookmark-meta-row">
              <div className="bookmark-source">
                <Favicon src={item.favicon_url} />
                <span className="bookmark-site">{item.site_name || domainFromURL(item.url)}</span>
              </div>
              <div className="bookmark-meta-right">
                <time className="bookmark-date tabular-nums" dateTime={item.created_at}>
                  {formatDate(item.created_at)}
                </time>
                {item.archived && <span className="status-badge">Archived</span>}
              </div>
            </div>

            <button
              type="button"
              className="bookmark-title-btn"
              onClick={() => onOpenDetail(item)}
            >
              {item.title || item.url}
            </button>

            {item.description && <p className="bookmark-desc">{item.description}</p>}
            {item.notes && <p className="bookmark-note-preview">{item.notes}</p>}

            <div className="bookmark-footer">
              <div className="pill-row">
                {item.lists.map((list) => (
                  <button
                    key={`l-${list.id}`}
                    type="button"
                    className="list-pill"
                    onClick={() => onSelectList(list.name)}
                  >
                    <FolderKanban size={12} aria-hidden="true" />
                    {list.name}
                  </button>
                ))}
                {item.tags.map((tag) => (
                  <button
                    key={`t-${tag.id}`}
                    type="button"
                    className="tag-pill"
                    onClick={() => onSelectTag(tag.name)}
                  >
                    <Hash size={12} aria-hidden="true" />
                    {tag.name}
                  </button>
                ))}
              </div>

              <div className="bookmark-actions">
                <a
                  href={item.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="btn btn-ghost btn-icon btn-icon-sm"
                  aria-label={`Open ${item.title || item.url} in new tab`}
                  title="Open original URL"
                >
                  <ExternalLink size={15} aria-hidden="true" />
                </a>
                <button
                  type="button"
                  className="btn btn-ghost btn-icon btn-icon-sm"
                  onClick={() => onToggleArchive(item)}
                  aria-label={item.archived ? "Unarchive bookmark" : "Archive bookmark"}
                  title={item.archived ? "Restore to active" : "Archive"}
                >
                  {item.archived ? (
                    <ArchiveRestore size={15} aria-hidden="true" />
                  ) : (
                    <Archive size={15} aria-hidden="true" />
                  )}
                </button>
              </div>
            </div>
          </div>
        </article>
      ))}
    </div>
  );
}
