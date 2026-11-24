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
  const inFlightRef = useRef(false);

  useEffect(() => {
    if (!authenticated) {
      initializedRef.current = false;
      lastSeenSeqRef.current = 0;
      wasActiveRef.current = false;
      lastActiveKeyRef.current = "";
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

        const ev = status.last_jev_event;
        if (!initializedRef.current) {
          initializedRef.current = true;
          lastSeenSeqRef.current = ev?.seq ?? 0;
          return;
        }

        if (!ev) {
          lastSeenSeqRef.current = 0;
          return;
        }

        if (ev.seq > 0 && ev.seq !== lastSeenSeqRef.current) {
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
