import { useState } from "react";
import { Archive, ArchiveRestore, ExternalLink, Globe, Hash, FolderKanban, Plus } from "lucide-react";
import type { Bookmark } from "../types";

interface BookmarkListProps {
  items: Bookmark[];
  loading: boolean;
  hasActiveFilters: boolean;
  onOpenDetail: (bookmark: Bookmark) => void;
  onToggleArchive: (bookmark: Bookmark) => void;
  onSelectTag: (tagName: string) => void;
  onSelectList: (listName: string) => void;
  onOpenAdd: () => void;
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
  hasActiveFilters,
  onOpenDetail,
  onToggleArchive,
  onSelectTag,
  onSelectList,
  onOpenAdd,
  onClearFilters,
}: BookmarkListProps) {
  if (loading && items.length === 0) {
    return (
      <div className="panel" aria-busy="true">
        <p style={{ margin: 0, color: "var(--fg-muted)", fontSize: 14 }}>Loading bookmarks...</p>
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <div className="empty-state">
        <h2 style={{ margin: 0, fontSize: 17, fontWeight: 600 }}>
          {hasActiveFilters ? "No matching bookmarks" : "Your vault is empty"}
        </h2>
        <p style={{ margin: 0, fontSize: 14, color: "var(--fg-muted)", maxWidth: 380 }}>
          {hasActiveFilters
            ? "Try a broader search term or clear your active tag, list, or archive filters."
            : "Paste any URL from X, Threads, or the web to fetch metadata and save it."}
        </p>
        <div style={{ display: "flex", gap: 10, marginTop: 6 }}>
          {hasActiveFilters ? (
            <button type="button" className="btn btn-secondary" onClick={onClearFilters}>
              Clear filters
            </button>
          ) : (
            <button type="button" className="btn btn-primary" onClick={onOpenAdd}>
              <Plus size={16} aria-hidden="true" />
              Add first URL
            </button>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="bookmark-list" role="list">
      {items.map((item) => (
        <article key={item.id} className="bookmark-card" role="listitem">
          <div className="bookmark-main">
            <div className="bookmark-head">
              <Favicon src={item.favicon_url} />
              <button
                type="button"
                className="bookmark-title-btn"
                onClick={() => onOpenDetail(item)}
              >
                {item.title || item.url}
              </button>
            </div>

            <div className="bookmark-domain">
              <span>{item.site_name || domainFromURL(item.url)}</span>
              <span aria-hidden="true">·</span>
              <time className="tabular-nums" dateTime={item.created_at}>
                {formatDate(item.created_at)}
              </time>
              {item.archived && (
                <>
                  <span aria-hidden="true">·</span>
                  <span style={{ color: "var(--accent-text)", fontWeight: 600 }}>Archived</span>
                </>
              )}
            </div>

            {item.description && <p className="bookmark-desc">{item.description}</p>}
            {item.notes && <p className="bookmark-note-preview">{item.notes}</p>}

            {(item.tags.length > 0 || item.lists.length > 0) && (
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
            )}
          </div>

          <div className="bookmark-actions">
            <a
              href={item.url}
              target="_blank"
              rel="noopener noreferrer"
              className="btn btn-ghost btn-icon"
              aria-label={`Open ${item.title || item.url} in new tab`}
              title="Open original URL"
            >
              <ExternalLink size={16} aria-hidden="true" />
            </a>
            <button
              type="button"
              className="btn btn-ghost btn-icon"
              onClick={() => onToggleArchive(item)}
              aria-label={item.archived ? "Unarchive bookmark" : "Archive bookmark"}
              title={item.archived ? "Unarchive" : "Archive"}
            >
              {item.archived ? (
                <ArchiveRestore size={16} aria-hidden="true" />
              ) : (
                <Archive size={16} aria-hidden="true" />
              )}
            </button>
          </div>
        </article>
      ))}
    </div>
  );
}
