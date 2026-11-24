import { useEffect, useRef } from "react";
import { api } from "../api";

const POLL_INTERVAL_MS = 5000;

type NotifyFn = (message: string, tone?: "success" | "error" | "info") => void;

export function useTaggingToasts(authenticated: boolean, notify: NotifyFn): void {
  const initializedRef = useRef(false);
  const mountedAtRef = useRef(Date.now());
  const lastSeenSeqRef = useRef(0);
  const wasActiveRef = useRef(false);
  const lastActiveKeyRef = useRef("");
  const wasBackfillRunningRef = useRef(false);
  const lastBackfillStartedAtRef = useRef("");
  const lastBackfillFinishedAtRef = useRef("");
  const inFlightRef = useRef(false);

  useEffect(() => {
    if (!authenticated) {
      initializedRef.current = false;
      lastSeenSeqRef.current = 0;
      wasActiveRef.current = false;
      lastActiveKeyRef.current = "";
      wasBackfillRunningRef.current = false;
      lastBackfillStartedAtRef.current = "";
      lastBackfillFinishedAtRef.current = "";
      inFlightRef.current = false;
      return;
    }

    mountedAtRef.current = Date.now();
    let disposed = false;
    let intervalId: number | null = null;
    let currentCtrl: AbortController | null = null;

    const pollOnce = async () => {
      if (disposed || document.hidden || inFlightRef.current) {
        return;
      }

      inFlightRef.current = true;
      const ctrl = new AbortController();
      currentCtrl = ctrl;

      try {
        const status = await api.taggingStatus(ctrl.signal);
        if (disposed) return;

        const activeIds = (status.active_jev_jobs ?? [])
          .map((j) => j.bookmark_id)
          .sort((a, b) => a - b);
        const activeKey = activeIds.join(",");

        if (activeIds.length > 0) {
          if (!wasActiveRef.current && activeKey !== lastActiveKeyRef.current) {
            notify("Tagging berjalan…", "info");
            lastActiveKeyRef.current = activeKey;
          }
          wasActiveRef.current = true;
        } else {
          wasActiveRef.current = false;
          lastActiveKeyRef.current = "";
        }

        const bf = status.last_backfill;
        if (!initializedRef.current) {
          initializedRef.current = true;
          lastSeenSeqRef.current = status.last_jev_event?.seq ?? 0;
          lastBackfillFinishedAtRef.current = bf?.finished_at ?? "";
          if (bf?.running) {
            if (bf.started_at !== lastBackfillStartedAtRef.current) {
              notify("Backfill berjalan…", "info");
              lastBackfillStartedAtRef.current = bf.started_at;
            }
            wasBackfillRunningRef.current = true;
          } else {
            wasBackfillRunningRef.current = false;
          }
          return;
        }

        if (bf?.running) {
          if (!wasBackfillRunningRef.current && bf.started_at !== lastBackfillStartedAtRef.current) {
            notify("Backfill berjalan…", "info");
            lastBackfillStartedAtRef.current = bf.started_at;
          }
          wasBackfillRunningRef.current = true;
        } else if (wasBackfillRunningRef.current) {
          wasBackfillRunningRef.current = false;
          if (bf?.finished_at && bf.finished_at !== lastBackfillFinishedAtRef.current) {
            lastBackfillFinishedAtRef.current = bf.finished_at;
            const tagged = (bf.result?.rules_tagged ?? 0) + (bf.result?.jev_tagged ?? 0);
            const queued = bf.result?.queued ?? 0;
            if (tagged === 0 && queued === 0) {
              notify("Backfill selesai — tidak ada yang ke-tag", "info");
            } else {
              notify(`Backfill selesai: ${tagged} ter-tag, ${queued} masuk antrean`, "success");
            }
          }
        } else if (bf?.finished_at) {
          lastBackfillFinishedAtRef.current = bf.finished_at;
        }

        const ev = status.last_jev_event;
        if (!ev) {
          lastSeenSeqRef.current = 0;
        } else if (ev.seq > 0 && ev.seq !== lastSeenSeqRef.current) {
          const parsedFinished = Date.parse(ev.finished_at);
          const isNew =
            ev.seq > lastSeenSeqRef.current ||
            (Number.isFinite(parsedFinished) && parsedFinished >= mountedAtRef.current);

          lastSeenSeqRef.current = ev.seq;

          if (isNew) {
            if (ev.error) {
              notify("Tagging gagal — coba lagi nanti", "error");
            } else if (ev.applied_tags && ev.applied_tags.length > 0) {
              notify(`Tag ditambahkan: ${ev.applied_tags.join(", ")}`, "success");
            } else if (ev.queued) {
              notify("Masuk antrean review (1)", "info");
            } else {
              notify("Tagging selesai — tidak ada tag yang yakin", "info");
            }
          }
        }
      } catch {
        // Silent failure: do not spam error toasts if network/server is down; next tick retries.
      } finally {
        if (currentCtrl === ctrl) {
          currentCtrl = null;
        }
        inFlightRef.current = false;
      }
    };

    const startPolling = () => {
      if (intervalId !== null || document.hidden || disposed) return;
      void pollOnce();
      intervalId = window.setInterval(() => {
        void pollOnce();
      }, POLL_INTERVAL_MS);
    };

    const stopPolling = () => {
      if (intervalId !== null) {
        window.clearInterval(intervalId);
        intervalId = null;
      }
      currentCtrl?.abort();
      currentCtrl = null;
      inFlightRef.current = false;
    };

    const onVisibilityChange = () => {
      if (document.hidden) {
        stopPolling();
      } else {
        startPolling();
      }
    };

    document.addEventListener("visibilitychange", onVisibilityChange);
    startPolling();

    return () => {
      disposed = true;
      document.removeEventListener("visibilitychange", onVisibilityChange);
      stopPolling();
    };
  }, [authenticated, notify]);
}
