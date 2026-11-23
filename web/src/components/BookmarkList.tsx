import { useEffect, useMemo, useRef, useState } from "react";
import {
  Archive,
  ArchiveRestore,
  Bookmark as BookmarkIcon,
  ExternalLink,
  FolderKanban,
  Globe,
  Hash,
  Plus,
  Upload,
} from "lucide-react";
import type { Bookmark, GridDensity } from "../types";
import { domainFromURL, formatDate, getDomainTone } from "../utils/bookmarks";

interface BookmarkListProps {
  items: Bookmark[];
  totalAvailable: number;
  loading: boolean;
  viewMode: "grid" | "list";
  density: GridDensity;
  selectedId: number | null;
  hasActiveFilters: boolean;
  hasMore: boolean;
  loadingMore: boolean;
  onSelectBookmark: (bookmark: Bookmark) => void;
  onToggleArchive: (bookmark: Bookmark) => void;
  onSelectTag: (tagName: string) => void;
  onSelectList: (listName: string) => void;
  onOpenAdd: () => void;
  onOpenImport?: () => void;
  onClearFilters: () => void;
  onLoadMore: () => void;
}

function Favicon({ src }: { src: string }) {
  const [failed, setFailed] = useState(false);
  if (!src || failed) {
    return (
      <span className="favicon-box" aria-hidden="true">
        <Globe size={12} />
      </span>
    );
  }
  return (
    <span className="favicon-box" aria-hidden="true">
      <img src={src} alt="" loading="lazy" onError={() => setFailed(true)} />
    </span>
  );
}

function CardThumbnail({
  bookmark,
  compact,
}: {
  bookmark: Bookmark;
  compact?: boolean;
}) {
  const [imgFailed, setImgFailed] = useState(false);
  const domain = bookmark.site_name || domainFromURL(bookmark.url);
  const tone = getDomainTone(domain);

  useEffect(() => {
    setImgFailed(false);
  }, [bookmark.image_url]);

  if (bookmark.image_url && !imgFailed) {
    return (
      <div className={`masonry-media ${compact ? "is-compact" : ""}`}>
        <img
          src={bookmark.image_url}
          alt={bookmark.title || domain}
          loading="lazy"
          decoding="async"
          onError={() => setImgFailed(true)}
        />
      </div>
    );
  }

  return (
    <div
      className={`masonry-fallback ${compact ? "is-compact" : ""}`}
      style={{ "--card-hue": tone.hue, "--card-tint": tone.accent } as React.CSSProperties}
    >
      <span className="masonry-fallback-monogram" aria-hidden="true">
        {tone.initial}
      </span>
      <div className="masonry-fallback-meta">
        <Favicon src={bookmark.favicon_url} />
        <span className="masonry-fallback-domain">{domain}</span>
      </div>
    </div>
  );
}

function computeColumnCount(width: number, density: GridDensity): number {
  if (width <= 0) return 3;
  if (width < 520) return 1;
  if (width < 800) return 2;
  if (width < 1100) return density === "compact" ? 4 : 3;
  if (width < 1420) return density === "compact" ? 5 : 4;
  return density === "compact" ? 6 : 5;
}

export function BookmarkList({
  items,
  loading,
  viewMode,
  density,
  selectedId,
  hasActiveFilters,
  hasMore,
  loadingMore,
  onSelectBookmark,
  onToggleArchive,
  onSelectTag,
  onSelectList,
  onOpenAdd,
  onOpenImport,
  onClearFilters,
  onLoadMore,
}: BookmarkListProps) {
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [containerWidth, setContainerWidth] = useState(0);

  useEffect(() => {
    const el = containerRef.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    setContainerWidth(el.clientWidth);
    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width);
      }
    });
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const columnCount = useMemo(
    () => computeColumnCount(containerWidth, density),
    [containerWidth, density]
  );

  const columns = useMemo(() => {
    const cols: Bookmark[][] = Array.from({ length: columnCount }, () => []);
    items.forEach((item, index) => {
      cols[index % columnCount].push(item);
    });
    return cols;
  }, [items, columnCount]);

  if (loading && items.length === 0) {
    return (
      <div ref={containerRef} className="masonry-loading-grid" aria-busy="true" aria-label="Loading bookmarks">
        {[0, 1, 2, 3, 4, 5].map((idx) => (
          <div key={idx} className="bookmark-skeleton glass-rim" aria-hidden="true">
            <div className={`skeleton-thumb ${idx % 2 === 0 ? "is-tall" : ""}`} />
            <div className="skeleton-line skeleton-line-short" />
            <div className="skeleton-line skeleton-line-title" />
          </div>
        ))}
      </div>
    );
  }

  if (items.length === 0) {
    return (
      <div ref={containerRef} className="empty-state glass-rim">
        <span className="empty-state-icon" aria-hidden="true">
          <BookmarkIcon size={22} />
        </span>
        <h2 className="empty-state-title">
          {hasActiveFilters ? "No matching references" : "Your vault is empty"}
        </h2>
        <p className="empty-state-desc">
          {hasActiveFilters
            ? "Try a broader search term or clear your active tag, list, or smart filters."
            : "Paste any URL from X, Threads, or the web to capture its thumbnail, metadata, and notes."}
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
    <div ref={containerRef} className="bookmark-stage">
      {viewMode === "grid" ? (
        <div
          className={`masonry-columns ${density === "compact" ? "is-compact" : ""}`}
          role="list"
          aria-label="Reference masonry grid"
        >
          {columns.map((col, colIdx) => (
            <div key={colIdx} className="masonry-col">
              {col.map((item) => {
                const domain = item.site_name || domainFromURL(item.url);
                const selected = item.id === selectedId;
                const hasImage = Boolean(item.image_url);
                return (
                  <article
                    key={item.id}
                    className={`masonry-card glass-rim ${item.archived ? "is-archived" : ""} ${selected ? "is-selected" : ""}`}
                    role="listitem"
                  >
                    <div className="masonry-card-actions" aria-label="Quick actions">
                      <a
                        href={item.url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="card-action-btn"
                        aria-label={`Open ${item.title || item.url} in new tab`}
                        title="Open URL"
                      >
                        <ExternalLink size={13} aria-hidden="true" />
                      </a>
                      <button
                        type="button"
                        className="card-action-btn"
                        onClick={() => onToggleArchive(item)}
                        aria-label={item.archived ? "Restore bookmark" : "Archive bookmark"}
                        title={item.archived ? "Restore" : "Archive"}
                      >
                        {item.archived ? (
                          <ArchiveRestore size={13} aria-hidden="true" />
                        ) : (
                          <Archive size={13} aria-hidden="true" />
                        )}
                      </button>
                    </div>

                    <button
                      type="button"
                      className="masonry-card-trigger"
                      onClick={() => onSelectBookmark(item)}
                      aria-pressed={selected}
                    >
                      <CardThumbnail bookmark={item} compact={density === "compact"} />

                      <div className="masonry-card-body">
                        <div className="bookmark-meta-row">
                          <div className="bookmark-source">
                            <Favicon src={item.favicon_url} />
                            <span className="bookmark-site">{domain}</span>
                          </div>
                          <div className="bookmark-meta-right">
                            <time className="bookmark-date tabular-nums" dateTime={item.created_at}>
                              {formatDate(item.created_at)}
                            </time>
                            {item.archived && <span className="status-badge">Archived</span>}
                          </div>
                        </div>

                        <span className="masonry-card-title">{item.title || item.url}</span>

                        {!hasImage && item.description && (
                          <p className="masonry-card-excerpt">{item.description}</p>
                        )}
                        {item.notes && <p className="bookmark-note-preview">{item.notes}</p>}
                      </div>
                    </button>

                    {(item.lists.length > 0 || item.tags.length > 0) && (
                      <div className="masonry-card-pills">
                        {item.lists.slice(0, 2).map((list) => (
                          <button
                            key={`l-${list.id}`}
                            type="button"
                            className="list-pill"
                            onClick={() => onSelectList(list.name)}
                          >
                            <FolderKanban size={11} aria-hidden="true" />
                            <span>{list.name}</span>
                          </button>
                        ))}
                        {item.tags.slice(0, 4).map((tag) => (
                          <button
                            key={`t-${tag.id}`}
                            type="button"
                            className="tag-pill"
                            onClick={() => onSelectTag(tag.name)}
                          >
                            <Hash size={11} aria-hidden="true" />
                            <span>{tag.name}</span>
                          </button>
                        ))}
                      </div>
                    )}
                  </article>
                );
              })}
            </div>
          ))}
        </div>
      ) : (
        <div className="bookmark-list" role="list" aria-label="Reference list">
          {items.map((item) => {
            const domain = item.site_name || domainFromURL(item.url);
            const selected = item.id === selectedId;
            return (
              <article
                key={item.id}
                className={`list-row-card glass-rim ${item.archived ? "is-archived" : ""} ${selected ? "is-selected" : ""}`}
                role="listitem"
              >
                <button
                  type="button"
                  className="list-row-trigger"
                  onClick={() => onSelectBookmark(item)}
                  aria-pressed={selected}
                >
                  <div className="list-row-thumb">
                    <CardThumbnail bookmark={item} compact />
                  </div>
                  <div className="list-row-content">
                    <div className="bookmark-meta-row">
                      <div className="bookmark-source">
                        <Favicon src={item.favicon_url} />
                        <span className="bookmark-site">{domain}</span>
                      </div>
                      <div className="bookmark-meta-right">
                        <time className="bookmark-date tabular-nums" dateTime={item.created_at}>
                          {formatDate(item.created_at)}
                        </time>
                        {item.archived && <span className="status-badge">Archived</span>}
                      </div>
                    </div>
                    <span className="list-row-title">{item.title || item.url}</span>
                    {item.description && <p className="bookmark-desc">{item.description}</p>}
                  </div>
                </button>

                <div className="list-row-trailing">
                  <div className="pill-row">
                    {item.lists.slice(0, 2).map((list) => (
                      <button
                        key={`l-${list.id}`}
                        type="button"
                        className="list-pill"
                        onClick={() => onSelectList(list.name)}
                      >
                        <FolderKanban size={11} aria-hidden="true" />
                        {list.name}
                      </button>
                    ))}
                    {item.tags.slice(0, 3).map((tag) => (
                      <button
                        key={`t-${tag.id}`}
                        type="button"
                        className="tag-pill"
                        onClick={() => onSelectTag(tag.name)}
                      >
                        <Hash size={11} aria-hidden="true" />
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
                      title="Open URL"
                    >
                      <ExternalLink size={14} aria-hidden="true" />
                    </a>
                    <button
                      type="button"
                      className="btn btn-ghost btn-icon btn-icon-sm"
                      onClick={() => onToggleArchive(item)}
                      aria-label={item.archived ? "Restore bookmark" : "Archive bookmark"}
                      title={item.archived ? "Restore" : "Archive"}
                    >
                      {item.archived ? (
                        <ArchiveRestore size={14} aria-hidden="true" />
                      ) : (
                        <Archive size={14} aria-hidden="true" />
                      )}
                    </button>
                  </div>
                </div>
              </article>
            );
          })}
        </div>
      )}

      {hasMore && (
        <div className="load-more-row">
          <button
            type="button"
            className="btn btn-glass"
            onClick={onLoadMore}
            disabled={loadingMore}
          >
            {loadingMore ? "Loading more..." : "Load more references"}
          </button>
        </div>
      )}
    </div>
  );
}
