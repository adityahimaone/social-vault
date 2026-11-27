import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { RotateCcw, X } from "lucide-react";
import { api, ApiError, setUnauthorizedHandler } from "./api";
import { AddBookmarkModal } from "./components/AddBookmarkModal";
import { BookmarkInspector } from "./components/BookmarkInspector";
import { BookmarkList } from "./components/BookmarkList";
import { DashboardToolbar } from "./components/DashboardToolbar";
import { ImportView } from "./components/ImportView";
import { ListsView } from "./components/ListsView";
import { LoginView } from "./components/LoginView";
import { QuickLookModal } from "./components/QuickLookModal";
import { ReviewQueueView } from "./components/ReviewQueueView";
import { Sidebar } from "./components/Sidebar";
import { StatsBar } from "./components/StatsBar";
import { TagsView } from "./components/TagsView";
import { ToastViewport, type ToastItem } from "./components/Toast";
import { useTaggingToasts } from "./hooks/useTaggingToasts";
import type {
  ActiveView,
  ArchivedFilter,
  Bookmark,
  GridDensity,
  GridStyle,
  LibrarySection,
  List,
  SmartFilter,
  SortMode,
  Stats,
  Tag,
  TaggingStatus,
} from "./types";
import { filterAndSortBookmarks, isWithinDays } from "./utils/bookmarks";

function readStorage<T extends string>(key: string, allowed: readonly T[], fallback: T): T {
  try {
    const saved = localStorage.getItem(key);
    if (saved && (allowed as readonly string[]).includes(saved)) {
      return saved as T;
    }
  } catch {
    // ignore storage restriction
  }
  return fallback;
}

function writeStorage(key: string, val: string) {
  try {
    localStorage.setItem(key, val);
  } catch {
    // ignore storage restriction
  }
}

function readFavoriteIds(): Set<number> {
  try {
    const raw = localStorage.getItem("sv_favorites");
    if (!raw) return new Set();
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed)) {
      return new Set(parsed.filter((n): n is number => typeof n === "number" && Number.isFinite(n)));
    }
  } catch {
    // ignore storage restriction
  }
  return new Set();
}

function writeFavoriteIds(ids: Set<number>) {
  writeStorage("sv_favorites", JSON.stringify(Array.from(ids)));
}

function getInitialTheme(): "dark" | "light" {
  try {
    if (localStorage.getItem("sv_theme") === "dark") return "dark";
  } catch {
    // ignore
  }
  return "light";
}

function useIsMobileInspector(): boolean {
  const [mobile, setMobile] = useState(() =>
    typeof window !== "undefined" ? window.innerWidth < 1200 : false
  );
  useEffect(() => {
    const mq = window.matchMedia("(max-width: 1199px)");
    const onChange = () => setMobile(mq.matches);
    onChange();
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);
  return mobile;
}

export function App() {
  const isMobileInspector = useIsMobileInspector();

  const [theme, setTheme] = useState<"dark" | "light">(getInitialTheme);
  const [viewMode, setViewMode] = useState<"grid" | "list">(() =>
    readStorage("sv_view_mode", ["grid", "list"] as const, "grid")
  );
  const [gridStyle, setGridStyle] = useState<GridStyle>(() =>
    readStorage("sv_grid_style", ["bento", "uniform"] as const, "bento")
  );
  const [sortMode, setSortMode] = useState<SortMode>(() =>
    readStorage("sv_sort_mode", ["newest", "oldest", "title"] as const, "newest")
  );
  const [density, setDensity] = useState<GridDensity>(() =>
    readStorage("sv_density", ["comfortable", "compact"] as const, "comfortable")
  );
  const [sidebarCollapsed, setSidebarCollapsed] = useState<boolean>(() =>
    readStorage("sv_sidebar_collapsed", ["true", "false"] as const, "false") === "true"
  );
  const [inspectorOpen, setInspectorOpen] = useState<boolean>(() =>
    readStorage("sv_inspector_open", ["true", "false"] as const, "true") === "true"
  );
  const [fullBleed, setFullBleed] = useState(false);

  const [authState, setAuthState] = useState<{
    loading: boolean;
    authenticated: boolean;
    passwordConfigured: boolean;
  }>({ loading: true, authenticated: false, passwordConfigured: true });

  const [activeView, setActiveView] = useState<ActiveView>("bookmarks");
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  const [librarySection, setLibrarySection] = useState<LibrarySection>("all");
  const [smartFilters, setSmartFilters] = useState<SmartFilter[]>([]);
  const [searchInput, setSearchInput] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [archivedFilter, setArchivedFilter] = useState<ArchivedFilter>("all");
  const [selectedTag, setSelectedTag] = useState("");
  const [selectedList, setSelectedList] = useState("");
  const [activeListId, setActiveListId] = useState<number | null>(null);
  const [nsfwLockedOverride, setNsfwLockedOverride] = useState<boolean | null>(null);

  const [bookmarks, setBookmarks] = useState<Bookmark[]>([]);
  const [totalBookmarks, setTotalBookmarks] = useState(0);
  const [loadingBookmarks, setLoadingBookmarks] = useState(false);
  const [loadingMore, setLoadingMore] = useState(false);
  const [stats, setStats] = useState<Stats | null>(null);
  const [tags, setTags] = useState<Tag[]>([]);
  const [lists, setLists] = useState<List[]>([]);
  const [taggingStatus, setTaggingStatus] = useState<TaggingStatus | null>(null);

  const [favoriteIds, setFavoriteIds] = useState<Set<number>>(readFavoriteIds);
  const [selectedBookmark, setSelectedBookmark] = useState<Bookmark | null>(null);
  const [quickLookBookmark, setQuickLookBookmark] = useState<Bookmark | null>(null);
  const [addModalOpen, setAddModalOpen] = useState(false);
  const [addModalInstant, setAddModalInstant] = useState(false);

  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const [toastPaused, setToastPaused] = useState(false);
  const toastIDRef = useRef(1);
  const sidebarSearchRef = useRef<HTMLInputElement | null>(null);
  const toolbarSearchRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    document.documentElement.style.colorScheme = theme;
    writeStorage("sv_theme", theme);
  }, [theme]);

  useEffect(() => writeStorage("sv_view_mode", viewMode), [viewMode]);
  useEffect(() => writeStorage("sv_grid_style", gridStyle), [gridStyle]);
  useEffect(() => writeStorage("sv_sort_mode", sortMode), [sortMode]);
  useEffect(() => writeStorage("sv_density", density), [density]);
  useEffect(
    () => writeStorage("sv_sidebar_collapsed", String(sidebarCollapsed)),
    [sidebarCollapsed]
  );
  useEffect(() => writeStorage("sv_inspector_open", String(inspectorOpen)), [inspectorOpen]);
  useEffect(() => writeFavoriteIds(favoriteIds), [favoriteIds]);

  const notify = useCallback(
    (
      message: string,
      tone: "success" | "error" | "info" = "info",
      action?: { label: string; onClick: () => void }
    ) => {
      const id = toastIDRef.current++;
      setToasts((prev) => [...prev.slice(-3), { id, message, tone, action }]);
    },
    []
  );

  useTaggingToasts(authState.authenticated, notify);

  useEffect(() => {
    if (toasts.length === 0 || toastPaused) return;
    const hasAction = Boolean(toasts[0]?.action);
    const timer = window.setTimeout(
      () => {
        if (document.hidden) return;
        setToasts((prev) => prev.slice(1));
      },
      hasAction ? 4800 : 3400
    );
    return () => window.clearTimeout(timer);
  }, [toasts, toastPaused]);

  useEffect(() => {
    setUnauthorizedHandler(() => {
      setAuthState((prev) => ({ ...prev, authenticated: false }));
    });
  }, []);

  useEffect(() => {
    let active = true;
    api
      .authMe()
      .then((res) => {
        if (!active) return;
        setAuthState({
          loading: false,
          authenticated: res.authenticated,
          passwordConfigured: res.password_configured,
        });
      })
      .catch(() => {
        if (!active) return;
        setAuthState({ loading: false, authenticated: false, passwordConfigured: true });
      });
    return () => {
      active = false;
    };
  }, []);

  useEffect(() => {
    const t = window.setTimeout(() => {
      setDebouncedQuery(searchInput.trim());
    }, 220);
    return () => window.clearTimeout(t);
  }, [searchInput]);

  const refreshMetadata = useCallback(async () => {
    const [s, t, l, ts] = await Promise.all([
      api.stats(),
      api.listTags(),
      api.listLists(),
      api.taggingStatus().catch(() => null),
    ]);
    setStats(s);
    setTags(t.items);
    setLists(l.items);
    if (ts) setTaggingStatus(ts);
  }, []);

  const refreshBookmarks = useCallback(
    async (signal?: AbortSignal) => {
      setLoadingBookmarks(true);
      try {
        const res = await api.listBookmarks(
          {
            q: debouncedQuery,
            archived: archivedFilter,
            tag: selectedTag,
            list: selectedList,
            untagged: librarySection === "untagged",
            limit: 200,
            offset: 0,
          },
          signal
        );
        setBookmarks(res.items);
        setTotalBookmarks(res.total);
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        notify(err instanceof ApiError ? err.message : "Failed to load bookmarks", "error");
      } finally {
        setLoadingBookmarks(false);
      }
    },
    [debouncedQuery, archivedFilter, selectedTag, selectedList, librarySection, notify]
  );

  async function handleLoadMore() {
    if (loadingMore || bookmarks.length >= totalBookmarks) return;
    setLoadingMore(true);
    try {
      const res = await api.listBookmarks({
        q: debouncedQuery,
        archived: archivedFilter,
        tag: selectedTag,
        list: selectedList,
        untagged: librarySection === "untagged",
        limit: 200,
        offset: bookmarks.length,
      });
      setBookmarks((prev) => {
        const seen = new Set(prev.map((b) => b.id));
        const next = res.items.filter((b) => !seen.has(b.id));
        return [...prev, ...next];
      });
      setTotalBookmarks(res.total);
    } catch (err) {
      notify(err instanceof ApiError ? err.message : "Failed to load more", "error");
    } finally {
      setLoadingMore(false);
    }
  }

  useEffect(() => {
    if (!authState.authenticated) return;
    void refreshMetadata();
  }, [authState.authenticated, refreshMetadata]);

  useEffect(() => {
    if (!authState.authenticated) return;
    const ctrl = new AbortController();
    void refreshBookmarks(ctrl.signal);
    return () => ctrl.abort();
  }, [authState.authenticated, refreshBookmarks]);

  const visibleBookmarks = useMemo(
    () =>
      filterAndSortBookmarks(bookmarks, {
        librarySection,
        smartFilters,
        sortMode,
        favoriteIds,
      }),
    [bookmarks, librarySection, smartFilters, sortMode, favoriteIds]
  );

  const libraryCounts = useMemo(() => {
    let untagged = 0;
    let recent = 0;
    let withThumb = 0;
    let favorites = 0;
    for (const b of bookmarks) {
      if (b.tags.length === 0) untagged++;
      if (isWithinDays(b.created_at, 7)) recent++;
      if (b.image_url) withThumb++;
      if (favoriteIds.has(b.id)) favorites++;
    }
    return {
      all: stats?.total ?? totalBookmarks,
      favorites,
      untagged: taggingStatus?.untagged_count ?? untagged,
      recent,
      archived: stats?.archived ?? 0,
      withThumb,
    };
  }, [bookmarks, stats, totalBookmarks, favoriteIds, taggingStatus]);

  const focusSearchInput = useCallback(() => {
    setActiveView("bookmarks");
    window.setTimeout(() => {
      if (sidebarCollapsed || fullBleed || window.innerWidth < 1024) {
        toolbarSearchRef.current?.focus();
      } else {
        sidebarSearchRef.current?.focus();
      }
    }, 0);
  }, [sidebarCollapsed, fullBleed]);

  useEffect(() => {
    if (!authState.authenticated) return;
    function onKeyDown(e: KeyboardEvent) {
      const target = e.target as HTMLElement | null;
      const isInput =
        target?.tagName === "INPUT" ||
        target?.tagName === "TEXTAREA" ||
        target?.tagName === "SELECT" ||
        target?.isContentEditable;
      if (isInput || addModalOpen || quickLookBookmark) return;

      if (e.key === "/") {
        e.preventDefault();
        focusSearchInput();
      } else if (e.key === "n") {
        e.preventDefault();
        setAddModalInstant(true);
        setAddModalOpen(true);
      } else if (e.key === " ") {
        if (
          target?.tagName !== "BUTTON" &&
          target?.tagName !== "A" &&
          selectedBookmark
        ) {
          e.preventDefault();
          setQuickLookBookmark(selectedBookmark);
        }
      } else if (e.key === "f") {
        e.preventDefault();
        setFullBleed((v) => !v);
      } else if (e.key === "i") {
        e.preventDefault();
        if (fullBleed) setFullBleed(false);
        setInspectorOpen((v) => !v);
      } else if (e.key === "[") {
        e.preventDefault();
        if (fullBleed) setFullBleed(false);
        setSidebarCollapsed((v) => !v);
      } else if (e.key === "Escape" && fullBleed) {
        e.preventDefault();
        setFullBleed(false);
      } else if ((e.key === "j" || e.key === "k") && activeView === "bookmarks" && visibleBookmarks.length > 0) {
        e.preventDefault();
        const idx = selectedBookmark
          ? visibleBookmarks.findIndex((b) => b.id === selectedBookmark.id)
          : -1;
        const nextIdx =
          e.key === "j"
            ? Math.min(visibleBookmarks.length - 1, idx + 1)
            : Math.max(0, idx <= 0 ? 0 : idx - 1);
        setSelectedBookmark(visibleBookmarks[nextIdx]);
        if (!isMobileInspector) setInspectorOpen(true);
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [
    authState.authenticated,
    addModalOpen,
    quickLookBookmark,
    fullBleed,
    activeView,
    visibleBookmarks,
    selectedBookmark,
    isMobileInspector,
    focusSearchInput,
  ]);

  function handleSelectLibrarySection(section: LibrarySection) {
    setLibrarySection(section);
    if (section === "archived") {
      setArchivedFilter("true");
    } else if (archivedFilter === "true") {
      setArchivedFilter("all");
    }
  }

  function handleToggleSmartFilter(sf: SmartFilter) {
    setSmartFilters((prev) =>
      prev.includes(sf) ? prev.filter((item) => item !== sf) : [...prev, sf]
    );
  }

  const handleNsfwStatusChange = useCallback((st: { configured: boolean; locked: boolean }) => {
    const locked = st.configured && st.locked;
    setNsfwLockedOverride(locked);
    if (locked) {
      setSelectedTag((prev) => (prev.toLowerCase() === "nsfw" ? "" : prev));
      setSelectedBookmark((prev) =>
        prev?.tags.some((t) => t.name.toLowerCase() === "nsfw") ? null : prev
      );
      setQuickLookBookmark((prev) =>
        prev?.tags.some((t) => t.name.toLowerCase() === "nsfw") ? null : prev
      );
    }
  }, []);

  async function handleLogout() {
    await api.logout();
    setAuthState((prev) => ({ ...prev, authenticated: false }));
    setSelectedBookmark(null);
    setQuickLookBookmark(null);
    setNsfwLockedOverride(null);
    setSelectedTag((prev) => (prev.toLowerCase() === "nsfw" ? "" : prev));
  }

  const applyArchiveState = useCallback(
    async (id: number, archived: boolean) => {
      const updated = await api.updateBookmark(id, { archived });
      setBookmarks((prev) => prev.map((b) => (b.id === updated.id ? updated : b)));
      setSelectedBookmark((prev) => (prev?.id === updated.id ? updated : prev));
      setQuickLookBookmark((prev) => (prev?.id === updated.id ? updated : prev));
      await refreshMetadata();
      return updated;
    },
    [refreshMetadata]
  );

  const handleToggleArchive = useCallback(
    async (item: Bookmark) => {
      const previousArchived = item.archived;
      try {
        const updated = await applyArchiveState(item.id, !previousArchived);
        notify(updated.archived ? "Archived" : "Restored", "success", {
          label: "Undo",
          onClick: () => {
            void applyArchiveState(item.id, previousArchived).catch((err) => {
              notify(err instanceof ApiError ? err.message : "Failed to undo", "error");
            });
          },
        });
      } catch (err) {
        notify(err instanceof ApiError ? err.message : "Failed to update bookmark", "error");
      }
    },
    [applyArchiveState, notify]
  );

  const handleToggleFavorite = useCallback(
    (item: Bookmark) => {
      const wasFavorite = favoriteIds.has(item.id);
      setFavoriteIds((prev) => {
        const next = new Set(prev);
        if (wasFavorite) next.delete(item.id);
        else next.add(item.id);
        return next;
      });
      notify(wasFavorite ? "Removed from favourites" : "Added to favourites", "success", {
        label: "Undo",
        onClick: () => {
          setFavoriteIds((prev) => {
            const next = new Set(prev);
            if (wasFavorite) next.add(item.id);
            else next.delete(item.id);
            return next;
          });
        },
      });
    },
    [favoriteIds, notify]
  );

  const handleToggleBookmarkList = useCallback(
    async (item: Bookmark, list: List) => {
      const inList = item.lists.some((l) => l.id === list.id);
      try {
        if (inList) {
          await api.removeBookmarkFromList(list.id, item.id);
        } else {
          await api.addBookmarksToList(list.id, [item.id]);
        }
        const nextLists = inList
          ? item.lists.filter((l) => l.id !== list.id)
          : [...item.lists, list];
        const updatedItem: Bookmark = { ...item, lists: nextLists };
        setBookmarks((prev) => prev.map((b) => (b.id === item.id ? updatedItem : b)));
        setSelectedBookmark((prev) => (prev?.id === item.id ? updatedItem : prev));
        setQuickLookBookmark((prev) => (prev?.id === item.id ? updatedItem : prev));
        await refreshMetadata();
        notify(inList ? `Removed from "${list.name}"` : `Added to "${list.name}"`, "success");
      } catch (err) {
        notify(err instanceof ApiError ? err.message : "Failed to update list membership", "error");
      }
    },
    [notify, refreshMetadata]
  );

  function clearFilters() {
    setSearchInput("");
    setDebouncedQuery("");
    setLibrarySection("all");
    setSmartFilters([]);
    setArchivedFilter("all");
    setSelectedTag("");
    setSelectedList("");
  }

  if (authState.loading) {
    return (
      <main className="login-page">
        <div className="loading-pill">
          <span className="loading-dot" aria-hidden="true" />
          <span>Loading Social Vault...</span>
        </div>
      </main>
    );
  }

  if (!authState.authenticated) {
    return (
      <LoginView
        passwordConfigured={authState.passwordConfigured}
        theme={theme}
        onToggleTheme={() => setTheme((t) => (t === "dark" ? "light" : "dark"))}
        onSuccess={() => setAuthState((prev) => ({ ...prev, authenticated: true }))}
      />
    );
  }

  const hasActiveFilters = Boolean(
    debouncedQuery ||
      archivedFilter !== "all" ||
      librarySection !== "all" ||
      smartFilters.length > 0 ||
      selectedTag ||
      selectedList
  );

  const effectiveSidebarCollapsed = fullBleed || sidebarCollapsed;
  const effectiveInspectorOpen =
    !fullBleed && (isMobileInspector ? Boolean(selectedBookmark) : inspectorOpen);
  const activeListName =
    activeListId != null ? lists.find((l) => l.id === activeListId)?.name ?? null : null;

  return (
    <div
      className={[
        "app-shell",
        effectiveSidebarCollapsed ? "is-sidebar-collapsed" : "",
        effectiveInspectorOpen && !isMobileInspector ? "is-inspector-open" : "",
        fullBleed ? "is-full-bleed" : "",
      ]
        .filter(Boolean)
        .join(" ")}
    >
      <a className="skip-link" href="#vault-main">
        Skip to bookmarks
      </a>

      <Sidebar
        open={mobileMenuOpen}
        collapsed={effectiveSidebarCollapsed}
        onClose={() => setMobileMenuOpen(false)}
        activeView={activeView}
        onSelectView={setActiveView}
        librarySection={librarySection}
        onSelectLibrarySection={handleSelectLibrarySection}
        smartFilters={smartFilters}
        onToggleSmartFilter={handleToggleSmartFilter}
        selectedTag={selectedTag}
        selectedList={selectedList}
        activeListId={activeListId}
        pendingReviewCount={taggingStatus?.pending_review ?? 0}
        onSelectTagFilter={setSelectedTag}
        onSelectListFilter={setSelectedList}
        onSelectListDetail={setActiveListId}
        searchInput={searchInput}
        onSearchChange={setSearchInput}
        searchInputRef={sidebarSearchRef}
        tags={tags}
        lists={lists}
        counts={libraryCounts}
        theme={theme}
        onToggleTheme={() => setTheme((t) => (t === "dark" ? "light" : "dark"))}
        onLogout={handleLogout}
        nsfwLockedOverride={nsfwLockedOverride}
      />

      <div className="main-area">
        <DashboardToolbar
          activeView={activeView}
          onSelectView={setActiveView}
          librarySection={librarySection}
          selectedTag={selectedTag}
          selectedList={selectedList}
          activeListName={activeListName}
          visibleCount={visibleBookmarks.length}
          totalCount={totalBookmarks}
          searchInput={searchInput}
          onSearchChange={setSearchInput}
          toolbarSearchRef={toolbarSearchRef}
          onFocusSearch={focusSearchInput}
          sidebarCollapsed={effectiveSidebarCollapsed}
          onToggleSidebar={() => {
            if (fullBleed) {
              setFullBleed(false);
              setSidebarCollapsed(false);
            } else {
              setSidebarCollapsed((v) => !v);
            }
          }}
          onOpenMobileMenu={() => setMobileMenuOpen(true)}
          inspectorOpen={inspectorOpen}
          onToggleInspector={() => {
            if (fullBleed) {
              setFullBleed(false);
              setInspectorOpen(true);
            } else {
              setInspectorOpen((v) => !v);
            }
          }}
          fullBleed={fullBleed}
          onToggleFullBleed={() => setFullBleed((v) => !v)}
          viewMode={viewMode}
          onChangeViewMode={setViewMode}
          gridStyle={gridStyle}
          onChangeGridStyle={setGridStyle}
          archivedFilter={archivedFilter}
          onChangeArchivedFilter={(f) => {
            setArchivedFilter(f);
            if (f !== "true" && librarySection === "archived") {
              setLibrarySection("all");
            }
          }}
          smartFilters={smartFilters}
          onToggleSmartFilter={handleToggleSmartFilter}
          sortMode={sortMode}
          onChangeSortMode={setSortMode}
          density={density}
          onChangeDensity={setDensity}
          theme={theme}
          onToggleTheme={() => setTheme((t) => (t === "dark" ? "light" : "dark"))}
          hasActiveFilters={hasActiveFilters}
          onClearFilters={clearFilters}
          onOpenAdd={() => {
            setAddModalInstant(false);
            setAddModalOpen(true);
          }}
          onLogout={handleLogout}
        />

        <main id="vault-main" className="content-container">
          {activeView === "bookmarks" && (
            <>
              {!fullBleed && (
                <StatsBar
                  stats={stats}
                  bookmarks={bookmarks}
                  activeArchivedFilter={archivedFilter}
                  selectedTag={selectedTag}
                  onSelectArchived={(f) => {
                    setArchivedFilter(f);
                    if (f === "true") setLibrarySection("archived");
                    else if (librarySection === "archived") setLibrarySection("all");
                  }}
                  onSelectTag={(name) =>
                    setSelectedTag((prev) =>
                      prev.toLowerCase() === name.toLowerCase() ? "" : name
                    )
                  }
                />
              )}

              {hasActiveFilters && (
                <div className="active-filters-bar" aria-label="Active filters">
                  {librarySection !== "all" && (
                    <button
                      type="button"
                      className="filter-chip"
                      onClick={() => handleSelectLibrarySection("all")}
                    >
                      <span>Section: {librarySection}</span>
                      <X size={12} aria-hidden="true" />
                    </button>
                  )}
                  {smartFilters.map((sf) => (
                    <button
                      key={sf}
                      type="button"
                      className="filter-chip"
                      onClick={() => handleToggleSmartFilter(sf)}
                    >
                      <span>{sf.replace("_", " ")}</span>
                      <X size={12} aria-hidden="true" />
                    </button>
                  ))}
                  {selectedList && (
                    <button
                      type="button"
                      className="filter-chip"
                      onClick={() => setSelectedList("")}
                    >
                      <span>List: {selectedList}</span>
                      <X size={12} aria-hidden="true" />
                    </button>
                  )}
                  {selectedTag && (
                    <button
                      type="button"
                      className="filter-chip"
                      onClick={() => setSelectedTag("")}
                    >
                      <span>#{selectedTag}</span>
                      <X size={12} aria-hidden="true" />
                    </button>
                  )}
                  {debouncedQuery && (
                    <button
                      type="button"
                      className="filter-chip"
                      onClick={() => {
                        setSearchInput("");
                        setDebouncedQuery("");
                      }}
                    >
                      <span>“{debouncedQuery}”</span>
                      <X size={12} aria-hidden="true" />
                    </button>
                  )}
                  <button
                    type="button"
                    className="btn btn-ghost btn-compact"
                    onClick={clearFilters}
                  >
                    <RotateCcw size={13} aria-hidden="true" />
                    Reset
                  </button>
                </div>
              )}

              <BookmarkList
                items={visibleBookmarks}
                lists={lists}
                totalAvailable={totalBookmarks}
                loading={loadingBookmarks}
                viewMode={viewMode}
                gridStyle={gridStyle}
                density={density}
                selectedId={selectedBookmark?.id ?? null}
                favoriteIds={favoriteIds}
                hasActiveFilters={hasActiveFilters}
                hasMore={bookmarks.length < totalBookmarks}
                loadingMore={loadingMore}
                onSelectBookmark={(b) => {
                  setSelectedBookmark(b);
                  if (!isMobileInspector && !fullBleed) {
                    setInspectorOpen(true);
                  }
                }}
                onQuickLook={(b) => setQuickLookBookmark(b)}
                onToggleFavorite={handleToggleFavorite}
                onToggleArchive={handleToggleArchive}
                onToggleBookmarkList={handleToggleBookmarkList}
                onOpenLists={() => {
                  setActiveListId(null);
                  setActiveView("lists");
                }}
                onSelectTag={(name) => setSelectedTag(name)}
                onSelectList={(name) => setSelectedList(name)}
                onOpenAdd={() => {
                  setAddModalInstant(false);
                  setAddModalOpen(true);
                }}
                onOpenImport={() => setActiveView("import")}
                onClearFilters={clearFilters}
                onLoadMore={handleLoadMore}
              />
            </>
          )}

          {activeView === "tags" && (
            <TagsView
              tags={tags}
              onRefresh={async () => {
                await Promise.all([refreshMetadata(), refreshBookmarks()]);
              }}
              onFilterByTag={(name) => {
                setSelectedTag(name);
                setActiveView("bookmarks");
              }}
              onOpenReview={() => setActiveView("review")}
              onNotify={notify}
              onNsfwStatusChange={handleNsfwStatusChange}
            />
          )}

          {activeView === "lists" && (
            <ListsView
              lists={lists}
              allTags={tags}
              activeListId={activeListId}
              onSelectListId={setActiveListId}
              onRefresh={async () => {
                await Promise.all([refreshMetadata(), refreshBookmarks()]);
              }}
              onSelectBookmark={(b) => {
                setSelectedBookmark(b);
                if (!isMobileInspector && !fullBleed) {
                  setInspectorOpen(true);
                }
              }}
              onNotify={notify}
            />
          )}

          {activeView === "review" && (
            <ReviewQueueView
              onRefreshMetadata={async () => {
                await Promise.all([refreshMetadata(), refreshBookmarks()]);
              }}
              onNotify={notify}
            />
          )}

          {activeView === "import" && (
            <ImportView
              onImported={async () => {
                await Promise.all([refreshMetadata(), refreshBookmarks()]);
              }}
              onNotify={notify}
            />
          )}
        </main>
      </div>

      <BookmarkInspector
        bookmark={selectedBookmark}
        open={effectiveInspectorOpen}
        isMobileViewport={isMobileInspector}
        isFavorite={selectedBookmark ? favoriteIds.has(selectedBookmark.id) : false}
        allLists={lists}
        stats={stats}
        withThumbnailCount={libraryCounts.withThumb}
        onClose={() => {
          if (isMobileInspector) {
            setSelectedBookmark(null);
          } else if (selectedBookmark) {
            setSelectedBookmark(null);
          } else {
            setInspectorOpen(false);
          }
        }}
        onSelectBookmark={(b) => setSelectedBookmark(b)}
        onSelectTagFilter={(name) => {
          setSelectedTag(name);
          setActiveView("bookmarks");
        }}
        onToggleFavorite={handleToggleFavorite}
        onToggleArchive={handleToggleArchive}
        onQuickLook={(b) => setQuickLookBookmark(b)}
        onUpdated={async (updated) => {
          setSelectedBookmark(updated);
          setQuickLookBookmark((prev) => (prev?.id === updated.id ? updated : prev));
          setBookmarks((prev) => prev.map((b) => (b.id === updated.id ? updated : b)));
          await refreshMetadata();
        }}
        onDeleted={async (id) => {
          setSelectedBookmark(null);
          setQuickLookBookmark((prev) => (prev?.id === id ? null : prev));
          setBookmarks((prev) => prev.filter((b) => b.id !== id));
          setTotalBookmarks((n) => Math.max(0, n - 1));
          await refreshMetadata();
        }}
        onNotify={notify}
      />

      <QuickLookModal
        bookmark={quickLookBookmark}
        isFavorite={quickLookBookmark ? favoriteIds.has(quickLookBookmark.id) : false}
        onClose={() => setQuickLookBookmark(null)}
        onToggleFavorite={handleToggleFavorite}
        onToggleArchive={handleToggleArchive}
        onInspect={(b) => {
          setSelectedBookmark(b);
          if (fullBleed) setFullBleed(false);
          if (!isMobileInspector) setInspectorOpen(true);
        }}
      />

      <AddBookmarkModal
        open={addModalOpen}
        instantOpen={addModalInstant}
        lists={lists}
        onClose={() => setAddModalOpen(false)}
        onCreated={async (created) => {
          setSelectedBookmark(created);
          await Promise.all([refreshMetadata(), refreshBookmarks()]);
        }}
        onNotify={notify}
      />

      <ToastViewport
        toasts={toasts}
        onDismiss={(id) => setToasts((prev) => prev.filter((t) => t.id !== id))}
        onPauseChange={setToastPaused}
      />
    </div>
  );
}
