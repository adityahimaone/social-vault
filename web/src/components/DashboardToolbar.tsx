import { useState, type RefObject } from "react";
import {
  ArrowLeft,
  ArrowUpDown,
  Check,
  LayoutDashboard,
  LayoutGrid,
  List as ListIcon,
  LogOut,
  Maximize2,
  Menu,
  Minimize2,
  Moon,
  PanelLeft,
  PanelRight,
  Plus,
  RotateCcw,
  Search,
  Settings,
  SlidersHorizontal,
  Sun,
  Upload,
  X,
} from "lucide-react";
import type {
  ActiveView,
  ArchivedFilter,
  GridDensity,
  GridStyle,
  LibrarySection,
  SmartFilter,
  SortMode,
} from "../types";
import { GlassPopover } from "./GlassPopover";
import { GlassSegmented } from "./glass/GlassSegmented";

interface DashboardToolbarProps {
  activeView: ActiveView;
  onSelectView: (view: ActiveView) => void;
  librarySection: LibrarySection;
  selectedTag: string;
  selectedList: string;
  activeListName?: string | null;
  visibleCount: number;
  totalCount: number;
  searchInput: string;
  onSearchChange: (val: string) => void;
  toolbarSearchRef: RefObject<HTMLInputElement | null>;
  onFocusSearch: () => void;
  sidebarCollapsed: boolean;
  onToggleSidebar: () => void;
  onOpenMobileMenu: () => void;
  inspectorOpen: boolean;
  onToggleInspector: () => void;
  fullBleed: boolean;
  onToggleFullBleed: () => void;
  viewMode: "grid" | "list";
  onChangeViewMode: (mode: "grid" | "list") => void;
  gridStyle: GridStyle;
  onChangeGridStyle: (style: GridStyle) => void;
  archivedFilter: ArchivedFilter;
  onChangeArchivedFilter: (f: ArchivedFilter) => void;
  smartFilters: SmartFilter[];
  onToggleSmartFilter: (sf: SmartFilter) => void;
  sortMode: SortMode;
  onChangeSortMode: (s: SortMode) => void;
  density: GridDensity;
  onChangeDensity: (d: GridDensity) => void;
  theme: "dark" | "light";
  onToggleTheme: () => void;
  hasActiveFilters: boolean;
  onClearFilters: () => void;
  onOpenAdd: () => void;
  onLogout: () => void;
}

const SECTION_LABELS: Record<LibrarySection, string> = {
  all: "All References",
  favorites: "Favourites",
  untagged: "Untagged",
  recent: "Recent (7d)",
  archived: "Archived",
};

const SMART_LABELS: { id: SmartFilter; label: string }[] = [
  { id: "this_week", label: "Added this week" },
  { id: "has_media", label: "With thumbnail" },
  { id: "with_notes", label: "With notes" },
  { id: "unlisted", label: "Not in a list" },
];

const SORT_LABELS: { id: SortMode; label: string }[] = [
  { id: "newest", label: "Newest first" },
  { id: "oldest", label: "Oldest first" },
  { id: "title", label: "Title (A–Z)" },
];

type LayoutChoice = "bento" | "uniform" | "list";

export function DashboardToolbar({
  activeView,
  onSelectView,
  librarySection,
  selectedTag,
  selectedList,
  activeListName,
  visibleCount,
  totalCount,
  searchInput,
  onSearchChange,
  toolbarSearchRef,
  onFocusSearch,
  sidebarCollapsed,
  onToggleSidebar,
  onOpenMobileMenu,
  inspectorOpen,
  onToggleInspector,
  fullBleed,
  onToggleFullBleed,
  viewMode,
  onChangeViewMode,
  gridStyle,
  onChangeGridStyle,
  archivedFilter,
  onChangeArchivedFilter,
  smartFilters,
  onToggleSmartFilter,
  sortMode,
  onChangeSortMode,
  density,
  onChangeDensity,
  theme,
  onToggleTheme,
  hasActiveFilters,
  onClearFilters,
  onOpenAdd,
  onLogout,
}: DashboardToolbarProps) {
  const [filterOpen, setFilterOpen] = useState(false);
  const [sortOpen, setSortOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);

  const viewTitle =
    activeView === "tags"
      ? "Tags"
      : activeView === "lists"
        ? activeListName || "Lists"
        : activeView === "review"
          ? "Review Queue"
          : activeView === "import"
            ? "Import"
            : selectedList
              ? selectedList
              : selectedTag
                ? `#${selectedTag}`
                : SECTION_LABELS[librarySection];

  const activeFilterCount =
    (archivedFilter !== "all" ? 1 : 0) +
    smartFilters.length +
    (selectedTag ? 1 : 0) +
    (selectedList ? 1 : 0);

  const layoutChoice: LayoutChoice = viewMode === "list" ? "list" : gridStyle;

  function handleLayoutChange(next: LayoutChoice) {
    if (next === "list") {
      onChangeViewMode("list");
    } else {
      onChangeGridStyle(next);
      onChangeViewMode("grid");
    }
  }

  return (
    <header className="topbar">
      <div className="topbar-left">
        <button
          type="button"
          className="btn btn-toolbar btn-icon mobile-menu-btn"
          onClick={onOpenMobileMenu}
          aria-label="Open navigation menu"
        >
          <Menu size={18} aria-hidden="true" />
        </button>

        <button
          type="button"
          className="btn btn-toolbar btn-icon desktop-only-btn"
          onClick={onToggleSidebar}
          aria-label={sidebarCollapsed || fullBleed ? "Show sidebar" : "Hide sidebar"}
          title={sidebarCollapsed || fullBleed ? "Show sidebar ([)" : "Hide sidebar ([)"}
          aria-pressed={!sidebarCollapsed && !fullBleed}
        >
          <PanelLeft size={16} aria-hidden="true" />
        </button>

        {activeView !== "bookmarks" && (
          <button
            type="button"
            className="btn btn-toolbar btn-compact"
            onClick={() => onSelectView("bookmarks")}
          >
            <ArrowLeft size={14} aria-hidden="true" />
            <span>Back</span>
          </button>
        )}

        <div className="toolbar-heading-wrap">
          <h1 className="toolbar-title">{viewTitle}</h1>
          {activeView === "bookmarks" && (
            <span className="items-counter-pill tabular-nums">
              {hasActiveFilters && visibleCount !== totalCount
                ? `${visibleCount} of ${totalCount}`
                : `${visibleCount}`}
            </span>
          )}
        </div>
      </div>

      <div className={`toolbar-search-slot ${sidebarCollapsed || fullBleed ? "is-expanded" : ""}`}>
        {sidebarCollapsed || fullBleed ? (
          <div className="search-wrap">
            <Search size={14} className="search-icon" aria-hidden="true" />
            <label className="sr-only" htmlFor="toolbar-vault-search">
              Search bookmarks
            </label>
            <input
              ref={toolbarSearchRef}
              id="toolbar-vault-search"
              type="search"
              className="input search-input"
              placeholder="Search titles, notes, URLs..."
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
                  toolbarSearchRef.current?.focus();
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
        ) : (
          <button
            type="button"
            className="btn btn-toolbar btn-compact toolbar-search-trigger"
            onClick={onFocusSearch}
            title="Focus search (/)"
          >
            <Search size={14} aria-hidden="true" />
            <span className="toolbar-label">Search</span>
            <span className="btn-kbd" aria-hidden="true">
              /
            </span>
          </button>
        )}
      </div>

      <div className="topbar-actions">
        <GlassPopover
          open={filterOpen}
          onClose={() => setFilterOpen(false)}
          label="Filter bookmarks"
          trigger={
            <button
              type="button"
              className={`btn btn-toolbar btn-compact ${activeFilterCount > 0 ? "is-active-nav" : ""}`}
              onClick={() => {
                setSortOpen(false);
                setSettingsOpen(false);
                setFilterOpen((v) => !v);
              }}
              aria-expanded={filterOpen}
            >
              <SlidersHorizontal size={14} aria-hidden="true" />
              <span className="toolbar-label">Filter</span>
              {activeFilterCount > 0 && (
                <span className="toolbar-badge tabular-nums">{activeFilterCount}</span>
              )}
            </button>
          }
        >
          <div className="popover-section">
            <div className="popover-title">Status</div>
            <div className="popover-chip-row">
              {(["all", "false", "true"] as ArchivedFilter[]).map((val) => {
                const label = val === "all" ? "All" : val === "false" ? "Active" : "Archived";
                return (
                  <button
                    key={val}
                    type="button"
                    className={`popover-chip ${archivedFilter === val ? "is-selected" : ""}`}
                    onClick={() => onChangeArchivedFilter(val)}
                  >
                    {label}
                  </button>
                );
              })}
            </div>
          </div>

          <div className="popover-section">
            <div className="popover-title">Smart filters</div>
            <div className="popover-list">
              {SMART_LABELS.map((item) => {
                const active = smartFilters.includes(item.id);
                return (
                  <button
                    key={item.id}
                    type="button"
                    className={`popover-row ${active ? "is-selected" : ""}`}
                    onClick={() => onToggleSmartFilter(item.id)}
                  >
                    <span>{item.label}</span>
                    {active && <Check size={14} aria-hidden="true" />}
                  </button>
                );
              })}
            </div>
          </div>

          {hasActiveFilters && (
            <div className="popover-footer">
              <button
                type="button"
                className="btn btn-ghost btn-compact btn-block"
                onClick={() => {
                  onClearFilters();
                  setFilterOpen(false);
                }}
              >
                <RotateCcw size={13} aria-hidden="true" />
                Reset all filters
              </button>
            </div>
          )}
        </GlassPopover>

        <GlassPopover
          open={sortOpen}
          onClose={() => setSortOpen(false)}
          label="Sort bookmarks"
          trigger={
            <button
              type="button"
              className="btn btn-toolbar btn-compact"
              onClick={() => {
                setFilterOpen(false);
                setSettingsOpen(false);
                setSortOpen((v) => !v);
              }}
              aria-expanded={sortOpen}
              title="Sort order"
            >
              <ArrowUpDown size={14} aria-hidden="true" />
              <span className="toolbar-label">Sort</span>
            </button>
          }
        >
          <div className="popover-section">
            <div className="popover-title">Sort by</div>
            <div className="popover-list">
              {SORT_LABELS.map((item) => {
                const active = sortMode === item.id;
                return (
                  <button
                    key={item.id}
                    type="button"
                    className={`popover-row ${active ? "is-selected" : ""}`}
                    onClick={() => {
                      onChangeSortMode(item.id);
                      setSortOpen(false);
                    }}
                  >
                    <span>{item.label}</span>
                    {active && <Check size={14} aria-hidden="true" />}
                  </button>
                );
              })}
            </div>
          </div>
        </GlassPopover>

        <GlassSegmented<LayoutChoice>
          ariaLabel="Bookmark layout mode"
          size="sm"
          value={layoutChoice}
          onChange={handleLayoutChange}
          options={[
            {
              value: "bento",
              label: <LayoutDashboard size={14} aria-hidden="true" />,
              ariaLabel: "Bento masonry grid",
            },
            {
              value: "uniform",
              label: <LayoutGrid size={14} aria-hidden="true" />,
              ariaLabel: "Uniform grid",
            },
            {
              value: "list",
              label: <ListIcon size={14} aria-hidden="true" />,
              ariaLabel: "List view",
            },
          ]}
        />

        <button
          type="button"
          className={`btn btn-toolbar btn-compact ${activeView === "import" ? "is-active-nav" : ""}`}
          aria-current={activeView === "import" ? "page" : undefined}
          onClick={() => onSelectView(activeView === "import" ? "bookmarks" : "import")}
          title="Import bookmarks"
        >
          <Upload size={14} aria-hidden="true" />
          <span className="topbar-btn-label">Import</span>
        </button>

        <button
          type="button"
          className={`btn btn-toolbar btn-icon btn-icon-sm desktop-only-btn ${fullBleed ? "is-active-nav" : ""}`}
          onClick={onToggleFullBleed}
          aria-pressed={fullBleed}
          aria-label={fullBleed ? "Exit full-bleed grid" : "Full-bleed grid"}
          title={fullBleed ? "Exit full-bleed (F)" : "Full-bleed grid (F)"}
        >
          {fullBleed ? <Minimize2 size={14} aria-hidden="true" /> : <Maximize2 size={14} aria-hidden="true" />}
        </button>

        <button
          type="button"
          className={`btn btn-toolbar btn-icon btn-icon-sm desktop-only-btn ${inspectorOpen && !fullBleed ? "is-active-nav" : ""}`}
          onClick={onToggleInspector}
          aria-pressed={inspectorOpen && !fullBleed}
          aria-label={inspectorOpen ? "Hide info panel" : "Show info panel"}
          title={inspectorOpen ? "Hide Info panel (I)" : "Show Info panel (I)"}
        >
          <PanelRight size={14} aria-hidden="true" />
        </button>

        <GlassPopover
          open={settingsOpen}
          onClose={() => setSettingsOpen(false)}
          label="Dashboard settings"
          trigger={
            <button
              type="button"
              className="btn btn-toolbar btn-icon btn-icon-sm"
              onClick={() => {
                setFilterOpen(false);
                setSortOpen(false);
                setSettingsOpen((v) => !v);
              }}
              aria-expanded={settingsOpen}
              aria-label="Settings"
              title="Settings"
            >
              <Settings size={14} aria-hidden="true" />
            </button>
          }
        >
          <div className="popover-section">
            <div className="popover-title">Appearance</div>
            <button type="button" className="popover-row" onClick={onToggleTheme}>
              <span>{theme === "dark" ? "Switch to Paper appearance" : "Switch to Dark appearance"}</span>
              {theme === "dark" ? <Sun size={14} aria-hidden="true" /> : <Moon size={14} aria-hidden="true" />}
            </button>
          </div>

          <div className="popover-section">
            <div className="popover-title">Grid density</div>
            <div className="popover-chip-row">
              {(["comfortable", "compact"] as GridDensity[]).map((d) => (
                <button
                  key={d}
                  type="button"
                  className={`popover-chip ${density === d ? "is-selected" : ""}`}
                  onClick={() => onChangeDensity(d)}
                >
                  {d === "comfortable" ? "Comfortable" : "Compact"}
                </button>
              ))}
            </div>
          </div>

          <div className="popover-section">
            <div className="popover-title">Shortcuts</div>
            <div className="popover-shortcuts">
              <div><kbd>/</kbd> Search</div>
              <div><kbd>N</kbd> Save URL</div>
              <div><kbd>Space</kbd> Quick look</div>
              <div><kbd>F</kbd> Full-bleed</div>
              <div><kbd>I</kbd> Info panel</div>
              <div><kbd>[</kbd> Sidebar</div>
              <div><kbd>J</kbd>/<kbd>K</kbd> Next / Prev</div>
            </div>
          </div>

          <div className="popover-footer">
            <button type="button" className="btn btn-ghost btn-compact btn-block" onClick={onLogout}>
              <LogOut size={13} aria-hidden="true" />
              Sign out
            </button>
          </div>
        </GlassPopover>

        <button type="button" className="btn btn-primary btn-compact-cta" onClick={onOpenAdd}>
          <Plus size={15} aria-hidden="true" />
          <span>Save URL</span>
          <span className="btn-kbd" aria-hidden="true">
            N
          </span>
        </button>
      </div>
    </header>
  );
}
