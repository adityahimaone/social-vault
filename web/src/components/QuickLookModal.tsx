import { useEffect, useRef, useState } from "react";
import {
  Archive,
  ArchiveRestore,
  ExternalLink,
  FolderKanban,
  Globe,
  Hash,
  PanelRight,
  Star,
  X,
} from "lucide-react";
import type { Bookmark } from "../types";
import { domainFromURL, formatDateTime } from "../utils/bookmarks";
import { useFocusTrap } from "./glass/useFocusTrap";

interface QuickLookModalProps {
  bookmark: Bookmark | null;
  isFavorite: boolean;
  onClose: () => void;
  onToggleFavorite: (bookmark: Bookmark) => void;
  onToggleArchive: (bookmark: Bookmark) => void;
  onInspect: (bookmark: Bookmark) => void;
}

export function QuickLookModal({
  bookmark,
  isFavorite,
  onClose,
  onToggleFavorite,
  onToggleArchive,
  onInspect,
}: QuickLookModalProps) {
  const panelRef = useRef<HTMLDivElement | null>(null);
  const [imgFailed, setImgFailed] = useState(false);

  const open = Boolean(bookmark);
  useFocusTrap(panelRef, open);

  useEffect(() => {
    setImgFailed(false);
  }, [bookmark?.id, bookmark?.image_url]);

  useEffect(() => {
    if (!open) return;
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape" || e.key === " ") {
        const target = e.target as HTMLElement | null;
        if (
          e.key === " " &&
          (target?.tagName === "BUTTON" || target?.tagName === "A")
        ) {
          return;
        }
        e.preventDefault();
        onClose();
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [open, onClose]);

  if (!bookmark) return null;

  const domain = bookmark.site_name || domainFromURL(bookmark.url);

  return (
    <div
      className="overlay"
      data-state="open"
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
    >
      <div
        ref={panelRef}
        className="modal-panel quicklook-panel"
        role="dialog"
        aria-modal="true"
        aria-labelledby="quicklook-heading"
      >
        <div className="quicklook-header">
          <div className="bookmark-source">
            <span className="favicon-box" aria-hidden="true">
              {bookmark.favicon_url ? (
                <img src={bookmark.favicon_url} alt="" />
              ) : (
                <Globe size={12} />
              )}
            </span>
            <span className="bookmark-site">{domain}</span>
            <span className="quicklook-dot" aria-hidden="true">·</span>
            <time className="bookmark-date tabular-nums" dateTime={bookmark.created_at}>
              {formatDateTime(bookmark.created_at)}
            </time>
            {bookmark.archived && <span className="status-badge">Archived</span>}
          </div>

          <button
            type="button"
            className="btn btn-ghost btn-icon btn-icon-sm"
            onClick={onClose}
            aria-label="Close Quick Look"
            title="Close (Esc)"
          >
            <X size={17} aria-hidden="true" />
          </button>
        </div>

        {bookmark.image_url && !imgFailed && (
          <div className="quicklook-media">
            <img
              src={bookmark.image_url}
              alt={bookmark.title || domain}
              onError={() => setImgFailed(true)}
            />
          </div>
        )}

        <div className="quicklook-body">
          <h2 id="quicklook-heading" className="quicklook-title">
            {bookmark.title || bookmark.url}
          </h2>
          <a
            href={bookmark.url}
            target="_blank"
            rel="noopener noreferrer"
            className="quicklook-url"
          >
            {bookmark.url}
          </a>

          {bookmark.description && (
            <p className="quicklook-desc">{bookmark.description}</p>
          )}

          {bookmark.notes && (
            <div className="quicklook-notes">{bookmark.notes}</div>
          )}

          {(bookmark.lists.length > 0 || bookmark.tags.length > 0) && (
            <div className="pill-row">
              {bookmark.lists.map((list) => (
                <span key={`l-${list.id}`} className="list-pill">
                  <FolderKanban size={11} aria-hidden="true" />
                  {list.name}
                </span>
              ))}
              {bookmark.tags.map((tag) => (
                <span key={`t-${tag.id}`} className="tag-pill">
                  <Hash size={11} aria-hidden="true" />
                  {tag.name}
                </span>
              ))}
            </div>
          )}
        </div>

        <div className="quicklook-footer">
          <div className="drawer-footer-left">
            <button
              type="button"
              className={`btn btn-secondary btn-compact ${isFavorite ? "is-selected-pill" : ""}`}
              aria-pressed={isFavorite}
              onClick={() => onToggleFavorite(bookmark)}
            >
              <Star
                size={13}
                aria-hidden="true"
                fill={isFavorite ? "currentColor" : "none"}
              />
              {isFavorite ? "Favourited" : "Favourite"}
            </button>
            <button
              type="button"
              className="btn btn-secondary btn-compact"
              onClick={() => onToggleArchive(bookmark)}
            >
              {bookmark.archived ? (
                <>
                  <ArchiveRestore size={13} aria-hidden="true" />
                  Restore
                </>
              ) : (
                <>
                  <Archive size={13} aria-hidden="true" />
                  Archive
                </>
              )}
            </button>
          </div>

          <div className="drawer-footer-left">
            <button
              type="button"
              className="btn btn-secondary btn-compact"
              onClick={() => {
                onInspect(bookmark);
                onClose();
              }}
            >
              <PanelRight size={13} aria-hidden="true" />
              Inspect &amp; edit
            </button>
            <a
              href={bookmark.url}
              target="_blank"
              rel="noopener noreferrer"
              className="btn btn-primary btn-compact"
            >
              <ExternalLink size={13} aria-hidden="true" />
              Open URL
            </a>
          </div>
        </div>
      </div>
    </div>
  );
}
