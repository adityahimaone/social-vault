import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Check,
  CheckCheck,
  ExternalLink,
  Hash,
  Loader2,
  Search,
  Sparkles,
  X,
} from "lucide-react";
import { api, ApiError, type TaggingStatus } from "../api";
import type { TagReviewItem } from "../types";
import { domainFromURL, formatDate } from "../utils/bookmarks";

interface ReviewQueueViewProps {
  onRefreshMetadata: () => Promise<void>;
  onNotify: (msg: string, tone: "success" | "error" | "info") => void;
}

export function ReviewQueueView({ onRefreshMetadata, onNotify }: ReviewQueueViewProps) {
  const [items, setItems] = useState<TagReviewItem[]>([]);
  const [total, setTotal] = useState(0);
  const [status, setStatus] = useState<TaggingStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [suggesting, setSuggesting] = useState(false);
  const [batchBusy, setBatchBusy] = useState(false);
  const [busyItemIds, setBusyItemIds] = useState<Set<number>>(new Set());

  const [filterText, setFilterText] = useState("");
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set());
  const [tagSelections, setTagSelections] = useState<Record<number, string[]>>({});

  const loadQueue = useCallback(
    async (signal?: AbortSignal) => {
      try {
        const [rev, st] = await Promise.all([
          api.listTagReview(200, signal),
          api.taggingStatus(signal),
        ]);
        if (signal?.aborted) return;
        setItems(rev.items);
        setTotal(rev.total);
        setStatus(st);
        setTagSelections((prev) => {
          const next: Record<number, string[]> = {};
          for (const item of rev.items) {
            next[item.id] = prev[item.id] ?? [...item.suggested_tags];
          }
          return next;
        });
        setSelectedIds((prev) => {
          const valid = new Set(rev.items.map((i) => i.id));
          const next = new Set<number>();
          for (const id of prev) {
            if (valid.has(id)) next.add(id);
          }
          return next;
        });
      } catch (err) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        onNotify(err instanceof ApiError ? err.message : "Failed to load review queue", "error");
      } finally {
        if (!signal?.aborted) setLoading(false);
      }
    },
    [onNotify]
  );

  useEffect(() => {
    const ctrl = new AbortController();
    void loadQueue(ctrl.signal);
    return () => ctrl.abort();
  }, [loadQueue]);

  const visibleItems = useMemo(() => {
    const q = filterText.trim().toLowerCase();
    if (!q) return items;
    return items.filter(
      (item) =>
        item.bookmark_title.toLowerCase().includes(q) ||
        item.bookmark_url.toLowerCase().includes(q) ||
        (item.bookmark_description ?? "").toLowerCase().includes(q) ||
        item.suggested_tags.some((t) => t.toLowerCase().includes(q))
    );
  }, [items, filterText]);

  const visibleIds = useMemo(() => visibleItems.map((i) => i.id), [visibleItems]);
  const allVisibleSelected =
    visibleIds.length > 0 && visibleIds.every((id) => selectedIds.has(id));

  function toggleSelectAllVisible() {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (allVisibleSelected) {
        for (const id of visibleIds) next.delete(id);
      } else {
        for (const id of visibleIds) next.add(id);
      }
      return next;
    });
  }

  function toggleItemSelected(id: number) {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function toggleTagChoice(itemId: number, tag: string) {
    setTagSelections((prev) => {
      const current = prev[itemId] ?? [];
      const exists = current.some((t) => t.toLowerCase() === tag.toLowerCase());
      const updated = exists
        ? current.filter((t) => t.toLowerCase() !== tag.toLowerCase())
        : [...current, tag];
      return { ...prev, [itemId]: updated };
    });
  }

  async function handleSuggestUntagged() {
    if (suggesting || !status?.jev_enabled || (status?.untagged_count ?? 0) === 0) return;
    setSuggesting(true);
    try {
      const res = await api.suggestUntagged(200);
      await Promise.all([loadQueue(), onRefreshMetadata()]);
      if (res.queued > 0) {
        onNotify(
          `Saran Jev siap direview: ${res.queued} masuk antrean (${res.processed} diproses, ${res.skipped} dilewati)`,
          "success"
        );
      } else {
        onNotify(
          `Jev selesai memeriksa ${res.processed} bookmark — tidak ada saran tag >= 0.5`,
          "info"
        );
      }
    } catch (err) {
      onNotify(
        err instanceof ApiError ? err.message : "Gagal meminta saran tag dari Jev",
        "error"
      );
    } finally {
      setSuggesting(false);
    }
  }

  async function handleApplyItem(item: TagReviewItem) {
    const chosen = tagSelections[item.id] ?? item.suggested_tags;
    if (chosen.length === 0) {
      onNotify("Pilih minimal 1 tag sebelum approve, atau klik Tolak", "info");
      return;
    }
    setBusyItemIds((prev) => new Set(prev).add(item.id));
    try {
      await api.applyTagReview(item.id, chosen);
      setItems((prev) => prev.filter((i) => i.id !== item.id));
      setTotal((n) => Math.max(0, n - 1));
      setSelectedIds((prev) => {
        const next = new Set(prev);
        next.delete(item.id);
        return next;
      });
      await Promise.all([onRefreshMetadata(), loadQueue()]);
      onNotify(`Tag diterapkan: ${chosen.map((t) => `#${t}`).join(", ")}`, "success");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Gagal menerapkan tag", "error");
    } finally {
      setBusyItemIds((prev) => {
        const next = new Set(prev);
        next.delete(item.id);
        return next;
      });
    }
  }

  async function handleDismissItem(item: TagReviewItem) {
    setBusyItemIds((prev) => new Set(prev).add(item.id));
    try {
      await api.dismissTagReview(item.id);
      setItems((prev) => prev.filter((i) => i.id !== item.id));
      setTotal((n) => Math.max(0, n - 1));
      setSelectedIds((prev) => {
        const next = new Set(prev);
        next.delete(item.id);
        return next;
      });
      await Promise.all([onRefreshMetadata(), loadQueue()]);
      onNotify("Saran tag ditolak", "info");
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Gagal menolak saran", "error");
    } finally {
      setBusyItemIds((prev) => {
        const next = new Set(prev);
        next.delete(item.id);
        return next;
      });
    }
  }

  async function handleBatch(action: "apply" | "dismiss", targetIds: number[]) {
    if (targetIds.length === 0 || batchBusy) return;
    setBatchBusy(true);
    try {
      const res = await api.batchTagReview(action, targetIds);
      await Promise.all([loadQueue(), onRefreshMetadata()]);
      onNotify(
        action === "apply"
          ? `Batch approve selesai: ${res.succeeded} diterapkan${res.skipped > 0 ? `, ${res.skipped} dilewati` : ""}`
          : `Batch tolak selesai: ${res.succeeded} ditolak`,
        res.succeeded > 0 ? "success" : "info"
      );
    } catch (err) {
      onNotify(err instanceof ApiError ? err.message : "Gagal memproses batch review", "error");
    } finally {
      setBatchBusy(false);
    }
  }

  const untaggedCount = status?.untagged_count ?? 0;
  const jevEnabled = Boolean(status?.jev_enabled);

  return (
    <section className="content-panel" aria-labelledby="review-heading">
      <div className="panel-header">
        <div>
          <h1 id="review-heading" className="panel-title">
            Review Queue
          </h1>
          <p className="panel-subtitle">
            Tinjau saran tag dari Jev sebelum diterapkan ke bookmark. Tidak ada tag yang diterapkan otomatis dari sini.
          </p>
        </div>

        <div className="panel-header-actions">
          <span className="panel-count-pill tabular-nums">{total} menunggu</span>
          <span className="panel-count-pill tabular-nums">{untaggedCount} belum ber-tag</span>
        </div>
      </div>

      <div className="autotag-card">
        <div className="autotag-header">
          <div>
            <h2 className="autotag-title">Saran tag untuk bookmark belum ber-tag</h2>
            <p className="autotag-subtitle">
              {jevEnabled
                ? "Jalankan Jev untuk menghasilkan saran tag pada bookmark tanpa tag dan masukkan ke antrean review di bawah (tanpa langsung apply)."
                : "Jev tidak aktif (JEV_API_KEY belum dikonfigurasi di server)."}
            </p>
          </div>
          <button
            type="button"
            className="btn btn-primary btn-compact"
            disabled={suggesting || !jevEnabled || untaggedCount === 0}
            onClick={handleSuggestUntagged}
          >
            {suggesting ? (
              <>
                <Loader2 size={14} className="spin" aria-hidden="true" />
                Meminta saran Jev…
              </>
            ) : (
              <>
                <Sparkles size={14} aria-hidden="true" />
                Sarankan tag untagged ({untaggedCount})
              </>
            )}
          </button>
        </div>
      </div>

      <div className="review-toolbar">
        <div className="search-wrap review-search">
          <Search size={14} className="search-icon" aria-hidden="true" />
          <input
            type="search"
            className="input search-input"
            placeholder="Filter antrean (judul, URL, saran tag)..."
            value={filterText}
            onChange={(e) => setFilterText(e.target.value)}
          />
          {filterText && (
            <button
              type="button"
              className="search-clear-btn"
              onClick={() => setFilterText("")}
              aria-label="Clear filter"
            >
              <X size={13} aria-hidden="true" />
            </button>
          )}
        </div>

        <div className="review-batch-actions">
          {visibleItems.length > 0 && (
            <>
              <label className="smart-check-row">
                <input
                  type="checkbox"
                  checked={allVisibleSelected}
                  onChange={toggleSelectAllVisible}
                />
                <span>Pilih semua ({visibleItems.length})</span>
              </label>

              {selectedIds.size > 0 && (
                <>
                  <button
                    type="button"
                    className="btn btn-secondary btn-compact"
                    disabled={batchBusy}
                    onClick={() => handleBatch("apply", Array.from(selectedIds))}
                  >
                    <Check size={14} aria-hidden="true" />
                    Approve pilihan ({selectedIds.size})
                  </button>
                  <button
                    type="button"
                    className="btn btn-ghost btn-compact"
                    disabled={batchBusy}
                    onClick={() => handleBatch("dismiss", Array.from(selectedIds))}
                  >
                    <X size={14} aria-hidden="true" />
                    Tolak pilihan ({selectedIds.size})
                  </button>
                </>
              )}

              <button
                type="button"
                className="btn btn-primary btn-compact"
                disabled={batchBusy}
                onClick={() => handleBatch("apply", visibleIds)}
              >
                <CheckCheck size={14} aria-hidden="true" />
                Approve semua yang tampil ({visibleItems.length})
              </button>
            </>
          )}
        </div>
      </div>

      {loading ? (
        <p className="panel-empty">Memuat antrean review…</p>
      ) : visibleItems.length === 0 ? (
        <p className="panel-empty">
          {items.length === 0
            ? "Antrean review kosong. Gunakan tombol “Sarankan tag untagged” di atas bila ada bookmark tanpa tag."
            : "Tidak ada item antrean yang cocok dengan pencarian."}
        </p>
      ) : (
        <div className="review-list">
          {visibleItems.map((item) => {
            const checked = selectedIds.has(item.id);
            const chosenTags = tagSelections[item.id] ?? item.suggested_tags;
            const isBusy = batchBusy || busyItemIds.has(item.id);
            const domain = item.bookmark_site_name || domainFromURL(item.bookmark_url);

            return (
              <article key={item.id} className={`review-card ${checked ? "is-selected" : ""}`}>
                <div className="review-card-head">
                  <div className="review-card-title-wrap">
                    <input
                      type="checkbox"
                      checked={checked}
                      disabled={isBusy}
                      onChange={() => toggleItemSelected(item.id)}
                      aria-label={`Select ${item.bookmark_title || item.bookmark_url}`}
                    />
                    <div className="review-card-copy">
                      <div className="review-card-meta">
                        <span className="bookmark-site">{domain}</span>
                        <span className="status-badge">{item.source}</span>
                        <time className="bookmark-date tabular-nums" dateTime={item.created_at}>
                          {formatDate(item.created_at)}
                        </time>
                      </div>
                      <a
                        href={item.bookmark_url}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="review-card-link"
                      >
                        <span>{item.bookmark_title || item.bookmark_url}</span>
                        <ExternalLink size={12} aria-hidden="true" />
                      </a>
                      {item.bookmark_description && (
                        <p className="review-card-desc">{item.bookmark_description}</p>
                      )}
                    </div>
                  </div>

                  <div className="review-item-actions">
                    <button
                      type="button"
                      className="btn btn-primary btn-compact"
                      disabled={isBusy || chosenTags.length === 0}
                      onClick={() => handleApplyItem(item)}
                    >
                      <Check size={14} aria-hidden="true" />
                      Approve
                    </button>
                    <button
                      type="button"
                      className="btn btn-secondary btn-compact"
                      disabled={isBusy}
                      onClick={() => handleDismissItem(item)}
                    >
                      <X size={14} aria-hidden="true" />
                      Tolak
                    </button>
                  </div>
                </div>

                <div className="review-tags-row" aria-label="Suggested tags">
                  {item.suggested_tags.map((tag) => {
                    const active = chosenTags.some((t) => t.toLowerCase() === tag.toLowerCase());
                    const score = item.jev_scores?.[tag];
                    const pct = typeof score === "number" ? `${Math.round(score * 100)}%` : null;
                    return (
                      <button
                        key={tag}
                        type="button"
                        className={`review-tag-chip ${active ? "is-selected" : "is-deselected"}`}
                        aria-pressed={active}
                        disabled={isBusy}
                        onClick={() => toggleTagChoice(item.id, tag)}
                        title={active ? "Klik untuk batalkan tag ini" : "Klik untuk sertakan tag ini"}
                      >
                        <Hash size={11} aria-hidden="true" />
                        <span>{tag}</span>
                        {pct && <span className="review-score-pill tabular-nums">{pct}</span>}
                      </button>
                    );
                  })}
                </div>
              </article>
            );
          })}
        </div>
      )}
    </section>
  );
}
