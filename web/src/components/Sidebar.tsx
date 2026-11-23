import type { RefObject } from "react";
import {
  Archive,
  Bookmark as BookmarkIcon,
  CalendarClock,
  FileText,
  FolderKanban,
  FolderOpen,
  Hash,
  Image as ImageIcon,
  LogOut,
  Moon,
  Search,
  Sparkles,
  Sun,
  Tag as TagIcon,
  Upload,
  X,
} from "lucide-react";
import type { ActiveView, LibrarySection, List, SmartFilter, Tag } from "../types";

interface SidebarProps {
  open: boolean;
  collapsed: boolean;
  onClose: () => void;
  activeView: ActiveView;
  onSelectView: (view: ActiveView) => void;
  librarySection: LibrarySection;
  onSelectLibrarySection: (section: LibrarySection) => void;
  smartFilters: SmartFilter[];
  onToggleSmartFilter: (filter: SmartFilter) => void;
  selectedTag: string;
  selectedList: string;
  onSelectTagFilter: (tagName: string) => void;
  onSelectListFilter: (listName: string) => void;
  searchInput: string;
  onSearchChange: (val: string) => void;
  searchInputRef: RefObject<HTMLInputElement | null>;
  tags: Tag[];
  lists: List[];
  counts: {
    all: number;
    untagged: number;
    recent: number;
    archived: number;
  };
  theme: "dark" | "light";
  onToggleTheme: () => void;
  onLogout: () => void;
}

const SMART_FILTER_ITEMS: { id: SmartFilter; label: string; icon: typeof CalendarClock }[] = [
  { id: "this_week", label: "This week", icon: CalendarClock },
  { id: "has_media", label: "With thumbnail", icon: ImageIcon },
  { id: "with_notes", label: "With notes", icon: FileText },
  { id: "unlisted", label: "Unlisted", icon: FolderOpen },
];

export function Sidebar({
  open,
  collapsed,
  onClose,
  activeView,
  onSelectView,
  librarySection,
  onSelectLibrarySection,
  smartFilters,
  onToggleSmartFilter,
  selectedTag,
  selectedList,
  onSelectTagFilter,
  onSelectListFilter,
  searchInput,
  onSearchChange,
  searchInputRef,
  tags,
  lists,
  counts,
  theme,
  onToggleTheme,
  onLogout,
}: SidebarProps) {
  function navigate(view: ActiveView) {
    onSelectView(view);
    onClose();
  }

  function pickSection(section: LibrarySection) {
    onSelectLibrarySection(section);
    navigate("bookmarks");
  }

  const isLibraryActive = activeView === "bookmarks" && !selectedTag && !selectedList;

  return (
    <>
      <div
        className={`mobile-backdrop ${open ? "is-open" : ""}`}
        onClick={onClose}
        aria-hidden="true"
      />
      <aside
        className={`sidebar glass-rim ${open ? "is-open" : ""} ${collapsed ? "is-collapsed" : ""}`}
        aria-label="Vault navigation"
      >
        <div className="brand-header">
          <div className="brand-mark">
            <span className="brand-badge" aria-hidden="true">
              <BookmarkIcon size={16} />
            </span>
            <div className="brand-copy">
              <span className="brand-name">Social Vault</span>
              <span className="brand-sub">Reference Library</span>
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

        <div className="sidebar-search-wrap">
          <Search size={15} className="search-icon" aria-hidden="true" />
          <label className="sr-only" htmlFor="sidebar-vault-search">
            Search bookmarks
          </label>
          <input
            ref={searchInputRef}
            id="sidebar-vault-search"
            type="search"
            className="input sidebar-search-input"
            placeholder="Search library..."
            value={searchInput}
            onChange={(e) => {
              onSearchChange(e.target.value);
              if (activeView !== "bookmarks") onSelectView("bookmarks");
            }}
          />
          {searchInput ? (
            <button
              type="button"
              className="search-clear-btn"
              onClick={() => {
                onSearchChange("");
                searchInputRef.current?.focus();
              }}
              aria-label="Clear search"
            >
              <X size={13} aria-hidden="true" />
            </button>
          ) : (
            <span className="search-kbd" aria-hidden="true">
              /
            </span>
          )}
        </div>

        <nav className="nav-section" aria-label="Library">
          <div className="nav-section-title">Library</div>

          <button
            type="button"
            className="nav-item"
            aria-current={isLibraryActive && librarySection === "all" ? "page" : undefined}
            onClick={() => {
              onSelectTagFilter("");
              onSelectListFilter("");
              pickSection("all");
            }}
          >
            <span className="nav-item-main">
              <BookmarkIcon size={15} aria-hidden="true" />
              <span className="nav-item-label">All</span>
            </span>
            <span className="nav-item-count">{counts.all}</span>
          </button>

          <button
            type="button"
            className="nav-item"
            aria-current={isLibraryActive && librarySection === "untagged" ? "page" : undefined}
            onClick={() => pickSection("untagged")}
          >
            <span className="nav-item-main">
              <TagIcon size={15} aria-hidden="true" />
              <span className="nav-item-label">Untagged</span>
            </span>
            <span className="nav-item-count">{counts.untagged}</span>
          </button>

          <button
            type="button"
            className="nav-item"
            aria-current={isLibraryActive && librarySection === "recent" ? "page" : undefined}
            onClick={() => pickSection("recent")}
          >
            <span className="nav-item-main">
              <Sparkles size={15} aria-hidden="true" />
              <span className="nav-item-label">Recent</span>
            </span>
            <span className="nav-item-count">{counts.recent}</span>
          </button>

          <button
            type="button"
            className="nav-item"
            aria-current={isLibraryActive && librarySection === "archived" ? "page" : undefined}
            onClick={() => pickSection("archived")}
          >
            <span className="nav-item-main">
              <Archive size={15} aria-hidden="true" />
              <span className="nav-item-label">Archived</span>
            </span>
            <span className="nav-item-count">{counts.archived}</span>
          </button>
        </nav>

        <div className="nav-section">
          <div className="nav-section-title">Smart Filters</div>
          {SMART_FILTER_ITEMS.map((sf) => {
            const Icon = sf.icon;
            const active = activeView === "bookmarks" && smartFilters.includes(sf.id);
            return (
              <button
                key={sf.id}
                type="button"
                className="nav-item"
                aria-pressed={active}
                onClick={() => {
                  onToggleSmartFilter(sf.id);
                  navigate("bookmarks");
                }}
              >
                <span className="nav-item-main">
                  <Icon size={15} aria-hidden="true" />
                  <span className="nav-item-label">{sf.label}</span>
                </span>
              </button>
            );
          })}
        </div>

        <div className="nav-section">
          <div className="nav-section-head">
            <span className="nav-section-title">Lists</span>
            <button
              type="button"
              className="nav-manage-btn"
              aria-current={activeView === "lists" ? "page" : undefined}
              onClick={() => navigate("lists")}
            >
              Manage ({lists.length})
            </button>
          </div>
          {lists.length > 0 ? (
            lists.slice(0, 8).map((list) => {
              const active =
                activeView === "bookmarks" && selectedList.toLowerCase() === list.name.toLowerCase();
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
            })
          ) : (
            <button type="button" className="nav-item" onClick={() => navigate("lists")}>
              <span className="nav-item-main">
                <FolderKanban size={15} aria-hidden="true" />
                <span className="nav-item-label">Create first list...</span>
              </span>
            </button>
          )}
        </div>

        <div className="nav-section">
          <div className="nav-section-head">
            <span className="nav-section-title">Tags</span>
            <button
              type="button"
              className="nav-manage-btn"
              aria-current={activeView === "tags" ? "page" : undefined}
              onClick={() => navigate("tags")}
            >
              Manage ({tags.length})
            </button>
          </div>
          {tags.length > 0 ? (
            tags.slice(0, 12).map((tag) => {
              const active =
                activeView === "bookmarks" && selectedTag.toLowerCase() === tag.name.toLowerCase();
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
            })
          ) : (
            <button type="button" className="nav-item" onClick={() => navigate("tags")}>
              <span className="nav-item-main">
                <Hash size={15} aria-hidden="true" />
                <span className="nav-item-label">Manage tags...</span>
              </span>
            </button>
          )}
        </div>

        <div className="sidebar-footer">
          <button
            type="button"
            className="nav-item"
            aria-current={activeView === "import" ? "page" : undefined}
            onClick={() => navigate("import")}
          >
            <span className="nav-item-main">
              <Upload size={15} aria-hidden="true" />
              <span className="nav-item-label">Import archive</span>
            </span>
          </button>
          <button type="button" className="nav-item" onClick={onToggleTheme}>
            <span className="nav-item-main">
              {theme === "dark" ? <Sun size={15} aria-hidden="true" /> : <Moon size={15} aria-hidden="true" />}
              <span className="nav-item-label">{theme === "dark" ? "Light glass" : "Dark glass"}</span>
            </span>
          </button>
          <button type="button" className="nav-item" onClick={onLogout}>
            <span className="nav-item-main">
              <LogOut size={15} aria-hidden="true" />
              <span className="nav-item-label">Sign out</span>
            </span>
          </button>
        </div>
      </aside>
    </>
  );
}
