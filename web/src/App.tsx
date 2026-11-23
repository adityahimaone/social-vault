import { useCallback, useEffect, useRef, useState } from "react";
import { LayoutGrid, List as ListIcon, Menu, Plus, RotateCcw, Search, Upload, X } from "lucide-react";
import { api, ApiError, setUnauthorizedHandler } from "./api";
import { AddBookmarkModal } from "./components/AddBookmarkModal";
import { BookmarkDrawer } from "./components/BookmarkDrawer";
import { BookmarkList } from "./components/BookmarkList";
import { AmbientBackdrop } from "./components/glass/AmbientBackdrop";
import { GlassSegmented } from "./components/glass/GlassSegmented";
import { ImportView } from "./components/ImportView";
import { ListsView } from "./components/ListsView";
import { LoginView } from "./components/LoginView";
import { Sidebar } from "./components/Sidebar";
import { StatsBar } from "./components/StatsBar";
import { TagsView } from "./components/TagsView";
import { ToastViewport, type ToastItem } from "./components/Toast";
import type { ActiveView, ArchivedFilter, Bookmark, List, Stats, Tag } from "./types";

function getInitialTheme(): "dark" | "light" {
  try {
    const saved = localStorage.getItem("sv_theme");
    if (saved === "light" || saved === "dark") return saved;
  } catch {
    // ignore storage restriction
  }
  return window.matchMedia?.("(prefers-color-scheme: light)").matches ? "light" : "dark";
}

function getInitialViewMode(): "grid" | "list" {
  try {
    const saved = localStorage.getItem("sv_view_mode");
    if (saved === "grid" || saved === "list") return saved;
  } catch {
    // ignore storage restriction
  }
  return "grid";
}

export function App() {
  const [theme, setTheme] = useState<"dark" | "light">(getInitialTheme);
  const [viewMode, setViewMode] = useState<"grid" | "list">(getInitialViewMode);
  const [authState, setAuthState] = useState<{
    loading: boolean;
    authenticated: boolean;
    passwordConfigured: boolean;
  }>({ loading: true, authenticated: false, passwordConfigured: true });

  const [activeView, setActiveView] = useState<ActiveView>("bookmarks");
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);

  const [searchInput, setSearchInput] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [archivedFilter, setArchivedFilter] = useState<ArchivedFilter>("all");
  const [selectedTag, setSelectedTag] = useState("");
  const [selectedList, setSelectedList] = useState("");

  const [bookmarks, setBookmarks] = useState<Bookmark[]>([]);
  const [totalBookmarks, setTotalBookmarks] = useState(0);
  const [loadingBookmarks, setLoadingBookmarks] = useState(false);
  const [stats, setStats] = useState<Stats | null>(null);
  const [tags, setTags] = useState<Tag[]>([]);
  const [lists, setLists] = useState<List[]>([]);

  const [selectedBookmark, setSelectedBookmark] = useState<Bookmark | null>(null);
  const [drawerInstant, setDrawerInstant] = useState(false);
  const [addModalOpen, setAddModalOpen] = useState(false);
  const [addModalInstant, setAddModalInstant] = useState(false);

  const [toasts, setToasts] = useState<ToastItem[]>([]);
  const [toastPaused, setToastPaused] = useState(false);
  const toastIDRef = useRef(1);
  const searchInputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    document.documentElement.setAttribute("data-theme", theme);
    document.documentElement.style.colorScheme = theme;
    try {
      localStorage.setItem("sv_theme", theme);
    } catch {
      // ignore storage restriction
    }
  }, [theme]);

  useEffect(() => {
    try {
      localStorage.setItem("sv_view_mode", viewMode);
    } catch {
      // ignore storage restriction
    }
  }, [viewMode]);

  const notify = useCallback((message: string, tone: "success" | "error" | "info" = "info") => {
    const id = toastIDRef.current++;
    setToasts((prev) => [...prev.slice(-3), { id, message, tone }]);
  }, []);

  useEffect(() => {
    if (toasts.length === 0 || toastPaused) return;
    const timer = window.setTimeout(() => {
      if (document.hidden) return;
      setToasts((prev) => prev.slice(1));
    }, 3400);
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
    }, 240);
    return () => window.clearTimeout(t);
  }, [searchInput]);

  const refreshMetadata = useCallback(async () => {
    const [s, t, l] = await Promise.all([api.stats(), api.listTags(), api.listLists()]);
    setStats(s);
    setTags(t.items);
    setLists(l.items);
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
            limit: 100,
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
    [debouncedQuery, archivedFilter, selectedTag, selectedList, notify]
  );

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

  useEffect(() => {
    if (!authState.authenticated) return;
    function onKeyDown(e: KeyboardEvent) {
      const target = e.target as HTMLElement | null;
      const isInput =
        target?.tagName === "INPUT" ||
        target?.tagName === "TEXTAREA" ||
        target?.tagName === "SELECT" ||
        target?.isContentEditable;
      if (isInput || addModalOpen || selectedBookmark) return;

      if (e.key === "/") {
        e.preventDefault();
        setActiveView("bookmarks");
        searchInputRef.current?.focus();
      } else if (e.key === "n") {
        e.preventDefault();
        setAddModalInstant(true);
        setAddModalOpen(true);
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [authState.authenticated, addModalOpen, selectedBookmark]);

  async function handleLogout() {
    await api.logout();
    setAuthState((prev) => ({ ...prev, authenticated: false }));
    setSelectedBookmark(null);
  }

  async function handleToggleArchive(item: Bookmark) {
    try {
      const updated = await api.updateBookmark(item.id, { archived: !item.archived });
      setBookmarks((prev) => prev.map((b) => (b.id === updated.id ? updated : b)));
      if (selectedBookmark?.id === updated.id) {
        setSelectedBookmark(updated);
      }
      await refreshMetadata();
      notify(updated.archived ? "Archived" : "Restored", "success");
    } catch (err) {
      notify(err instanceof ApiError ? err.message : "Failed to update bookmark", "error");
    }
  }

  function clearFilters() {
    setSearchInput("");
    setDebouncedQuery("");
    setArchivedFilter("all");
    setSelectedTag("");
    setSelectedList("");
  }

  if (authState.loading) {
    return (
      <main className="login-page">
        <AmbientBackdrop />
        <div className="loading-pill glass-rim">
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
    debouncedQuery || archivedFilter !== "all" || selectedTag || selectedList
  );

  return (
    <div className="app-shell">
      <AmbientBackdrop />

      <Sidebar
        open={mobileMenuOpen}
        onClose={() => setMobileMenuOpen(false)}
        activeView={activeView}
        onSelectView={setActiveView}
        selectedTag={selectedTag}
        selectedList={selectedList}
        onSelectTagFilter={setSelectedTag}
        onSelectListFilter={setSelectedList}
        tags={tags}
        lists={lists}
        totalBookmarks={stats?.total ?? totalBookmarks}
        theme={theme}
        onToggleTheme={() => setTheme((t) => (t === "dark" ? "light" : "dark"))}
        onLogout={handleLogout}
      />

      <div className="main-area">
        <header className="topbar glass-rim">
          <button
            type="button"
            className="btn btn-ghost btn-icon mobile-menu-btn"
            onClick={() => setMobileMenuOpen(true)}
            aria-label="Open navigation menu"
          >
            <Menu size={18} aria-hidden="true" />
          </button>

          <div className="search-wrap">
            <Search size={16} className="search-icon" aria-hidden="true" />
            <label className="sr-only" htmlFor="vault-search">
              Search bookmarks
            </label>
            <input
              ref={searchInputRef}
              id="vault-search"
              type="search"
              className="input search-input"
              placeholder="Search titles, notes, URLs..."
              value={searchInput}
              onChange={(e) => {
                setSearchInput(e.target.value);
                if (activeView !== "bookmarks") setActiveView("bookmarks");
              }}
            />
            {searchInput ? (
              <button
                type="button"
                className="search-clear-btn"
                onClick={() => {
                  setSearchInput("");
                  setDebouncedQuery("");
                  searchInputRef.current?.focus();
                }}
                aria-label="Clear search"
              >
                <X size={14} aria-hidden="true" />
              </button>
            ) : (
              <span className="search-kbd" aria-hidden="true">
                /
              </span>
            )}
          </div>

          <div className="topbar-actions">
            <button
              type="button"
              className={`btn btn-glass ${activeView === "import" ? "is-active-nav" : ""}`}
              aria-current={activeView === "import" ? "page" : undefined}
              onClick={() => setActiveView("import")}
            >
              <Upload size={15} aria-hidden="true" />
              <span className="topbar-btn-label">Import</span>
            </button>

            <button
              type="button"
              className="btn btn-primary"
              onClick={() => {
                setAddModalInstant(false);
                setAddModalOpen(true);
              }}
            >
              <Plus size={16} aria-hidden="true" />
              <span>Save URL</span>
              <span className="btn-kbd" aria-hidden="true">
                N
              </span>
            </button>
          </div>
        </header>

        <main className="content-container">
          {activeView === "bookmarks" && (
            <>
              <StatsBar
                stats={stats}
                activeArchivedFilter={archivedFilter}
                selectedTag={selectedTag}
                onSelectArchived={(f) => setArchivedFilter(f)}
                onSelectTag={(name) =>
                  setSelectedTag((prev) =>
                    prev.toLowerCase() === name.toLowerCase() ? "" : name
                  )
                }
              />

              <div className="toolbar">
                <div className="toolbar-left">
                  <GlassSegmented
                    ariaLabel="Archive status filter"
                    value={archivedFilter}
                    onChange={setArchivedFilter}
                    options={[
                      { value: "all", label: "All" },
                      { value: "false", label: "Active" },
                      { value: "true", label: "Archived" },
                    ]}
                  />

                  <div className="active-filters">
                    {selectedList && (
                      <button
                        type="button"
                        className="filter-chip"
                        onClick={() => setSelectedList("")}
                      >
                        <span>List: {selectedList}</span>
                        <X size={13} aria-hidden="true" />
                      </button>
                    )}
                    {selectedTag && (
                      <button
                        type="button"
                        className="filter-chip"
                        onClick={() => setSelectedTag("")}
                      >
                        <span>#{selectedTag}</span>
                        <X size={13} aria-hidden="true" />
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
                        <X size={13} aria-hidden="true" />
                      </button>
                    )}
                    {hasActiveFilters && (
                      <button
                        type="button"
                        className="btn btn-ghost btn-compact"
                        onClick={clearFilters}
                      >
                        <RotateCcw size={13} aria-hidden="true" />
                        Reset
                      </button>
                    )}
                  </div>
                </div>

                <div className="toolbar-right">
                  <span className="result-count tabular-nums">
                    {totalBookmarks} {totalBookmarks === 1 ? "result" : "results"}
                  </span>

                  <GlassSegmented
                    ariaLabel="Bookmark layout mode"
                    size="sm"
                    value={viewMode}
                    onChange={setViewMode}
                    options={[
                      {
                        value: "grid",
                        label: <LayoutGrid size={15} aria-hidden="true" />,
                        ariaLabel: "Grid view",
                      },
                      {
                        value: "list",
                        label: <ListIcon size={15} aria-hidden="true" />,
                        ariaLabel: "List view",
                      },
                    ]}
                  />
                </div>
              </div>

              <BookmarkList
                items={bookmarks}
                loading={loadingBookmarks}
                viewMode={viewMode}
                hasActiveFilters={hasActiveFilters}
                onOpenDetail={(b) => {
                  setDrawerInstant(false);
                  setSelectedBookmark(b);
                }}
                onToggleArchive={handleToggleArchive}
                onSelectTag={(name) => setSelectedTag(name)}
                onSelectList={(name) => setSelectedList(name)}
                onOpenAdd={() => {
                  setAddModalInstant(false);
                  setAddModalOpen(true);
                }}
                onOpenImport={() => setActiveView("import")}
                onClearFilters={clearFilters}
              />
            </>
          )}

          {activeView === "tags" && (
            <TagsView
              tags={tags}
              onRefresh={refreshMetadata}
              onFilterByTag={(name) => {
                setSelectedTag(name);
                setActiveView("bookmarks");
              }}
              onNotify={notify}
            />
          )}

          {activeView === "lists" && (
            <ListsView
              lists={lists}
              onRefresh={refreshMetadata}
              onFilterByList={(name) => {
                setSelectedList(name);
                setActiveView("bookmarks");
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

      <AddBookmarkModal
        open={addModalOpen}
        instantOpen={addModalInstant}
        lists={lists}
        onClose={() => setAddModalOpen(false)}
        onCreated={async () => {
          await Promise.all([refreshMetadata(), refreshBookmarks()]);
        }}
        onNotify={notify}
      />

      <BookmarkDrawer
        bookmark={selectedBookmark}
        instantOpen={drawerInstant}
        allLists={lists}
        onClose={() => setSelectedBookmark(null)}
        onUpdated={async (updated) => {
          setSelectedBookmark(updated);
          setBookmarks((prev) => prev.map((b) => (b.id === updated.id ? updated : b)));
          await refreshMetadata();
        }}
        onDeleted={async (id) => {
          setBookmarks((prev) => prev.filter((b) => b.id !== id));
          setTotalBookmarks((n) => Math.max(0, n - 1));
          await refreshMetadata();
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
