import {
  Bookmark as BookmarkIcon,
  FolderKanban,
  Hash,
  LogOut,
  Moon,
  Sun,
  Upload,
  X,
} from "lucide-react";
import type { ActiveView, List, Tag } from "../types";

interface SidebarProps {
  open: boolean;
  onClose: () => void;
  activeView: ActiveView;
  onSelectView: (view: ActiveView) => void;
  selectedTag: string;
  selectedList: string;
  onSelectTagFilter: (tagName: string) => void;
  onSelectListFilter: (listName: string) => void;
  tags: Tag[];
  lists: List[];
  totalBookmarks: number;
  theme: "dark" | "light";
  onToggleTheme: () => void;
  onLogout: () => void;
}

export function Sidebar({
  open,
  onClose,
  activeView,
  onSelectView,
  selectedTag,
  selectedList,
  onSelectTagFilter,
  onSelectListFilter,
  tags,
  lists,
  totalBookmarks,
  theme,
  onToggleTheme,
  onLogout,
}: SidebarProps) {
  function navigate(view: ActiveView) {
    onSelectView(view);
    onClose();
  }

  return (
    <>
      <div
        className={`mobile-backdrop ${open ? "is-open" : ""}`}
        onClick={onClose}
        aria-hidden="true"
      />
      <aside className={`sidebar glass-rim ${open ? "is-open" : ""}`} aria-label="Vault navigation">
        <div className="brand-header">
          <div className="brand-mark">
            <span className="brand-badge" aria-hidden="true">
              <BookmarkIcon size={16} />
            </span>
            <div className="brand-copy">
              <span className="brand-name">Social Vault</span>
              <span className="brand-sub">Archive</span>
            </div>
          </div>
          <button
            type="button"
            className="btn btn-ghost btn-icon mobile-menu-btn"
            onClick={onClose}
            aria-label="Close navigation menu"
          >
            <X size={18} aria-hidden="true" />
          </button>
        </div>

        <nav className="nav-section" aria-label="Primary">
          <button
            type="button"
            className="nav-item"
            aria-current={activeView === "bookmarks" && !selectedTag && !selectedList ? "page" : undefined}
            onClick={() => {
              onSelectTagFilter("");
              onSelectListFilter("");
              navigate("bookmarks");
            }}
          >
            <span className="nav-item-main">
              <BookmarkIcon size={16} aria-hidden="true" />
              <span className="nav-item-label">All Bookmarks</span>
            </span>
            <span className="nav-item-count">{totalBookmarks}</span>
          </button>

          <button
            type="button"
            className="nav-item"
            aria-current={activeView === "tags" ? "page" : undefined}
            onClick={() => navigate("tags")}
          >
            <span className="nav-item-main">
              <Hash size={16} aria-hidden="true" />
              <span className="nav-item-label">Tags</span>
            </span>
            <span className="nav-item-count">{tags.length}</span>
          </button>

          <button
            type="button"
            className="nav-item"
            aria-current={activeView === "lists" ? "page" : undefined}
            onClick={() => navigate("lists")}
          >
            <span className="nav-item-main">
              <FolderKanban size={16} aria-hidden="true" />
              <span className="nav-item-label">Lists</span>
            </span>
            <span className="nav-item-count">{lists.length}</span>
          </button>

          <button
            type="button"
            className="nav-item"
            aria-current={activeView === "import" ? "page" : undefined}
            onClick={() => navigate("import")}
          >
            <span className="nav-item-main">
              <Upload size={16} aria-hidden="true" />
              <span className="nav-item-label">Import</span>
            </span>
          </button>
        </nav>

        {lists.length > 0 && (
          <div className="nav-section">
            <div className="nav-section-title">Lists</div>
            {lists.slice(0, 8).map((list) => {
              const active = activeView === "bookmarks" && selectedList.toLowerCase() === list.name.toLowerCase();
              return (
                <button
                  key={list.id}
                  type="button"
                  className="nav-item"
                  aria-pressed={active}
                  onClick={() => {
                    onSelectListFilter(active ? "" : list.name);
                    navigate("bookmarks");
                  }}
                >
                  <span className="nav-item-main">
                    <FolderKanban size={15} aria-hidden="true" />
                    <span className="nav-item-label">{list.name}</span>
                  </span>
                  <span className="nav-item-count">{list.count ?? 0}</span>
                </button>
              );
            })}
          </div>
        )}

        {tags.length > 0 && (
          <div className="nav-section">
            <div className="nav-section-title">Top Tags</div>
            {tags.slice(0, 10).map((tag) => {
              const active = activeView === "bookmarks" && selectedTag.toLowerCase() === tag.name.toLowerCase();
              return (
                <button
                  key={tag.id}
                  type="button"
                  className="nav-item"
                  aria-pressed={active}
                  onClick={() => {
                    onSelectTagFilter(active ? "" : tag.name);
                    navigate("bookmarks");
                  }}
                >
                  <span className="nav-item-main">
                    <Hash size={15} aria-hidden="true" />
                    <span className="nav-item-label">{tag.name}</span>
                  </span>
                  <span className="nav-item-count">{tag.count ?? 0}</span>
                </button>
              );
            })}
          </div>
        )}

        <div className="sidebar-footer">
          <button type="button" className="nav-item" onClick={onToggleTheme}>
            <span className="nav-item-main">
              {theme === "dark" ? <Sun size={16} aria-hidden="true" /> : <Moon size={16} aria-hidden="true" />}
              <span className="nav-item-label">{theme === "dark" ? "Light glass" : "Dark glass"}</span>
            </span>
          </button>
          <button type="button" className="nav-item" onClick={onLogout}>
            <span className="nav-item-main">
              <LogOut size={16} aria-hidden="true" />
              <span className="nav-item-label">Sign out</span>
            </span>
          </button>
        </div>
      </aside>
    </>
  );
}
