import { CheckCircle2, AlertCircle, X } from "lucide-react";

export interface ToastItem {
  id: number;
  message: string;
  tone: "success" | "error" | "info";
}

interface ToastViewportProps {
  toasts: ToastItem[];
  onDismiss: (id: number) => void;
  onPauseChange: (paused: boolean) => void;
}

export function ToastViewport({ toasts, onDismiss, onPauseChange }: ToastViewportProps) {
  if (toasts.length === 0) return null;

  return (
    <div
      className="toast-viewport"
      role="status"
      aria-live="polite"
      onMouseEnter={() => onPauseChange(true)}
      onMouseLeave={() => onPauseChange(false)}
    >
      {toasts.map((t) => (
        <div key={t.id} className="toast" data-tone={t.tone}>
          <div style={{ display: "flex", alignItems: "center", gap: 8, minWidth: 0 }}>
            {t.tone === "error" ? (
              <AlertCircle size={16} aria-hidden="true" style={{ color: "var(--destructive-text)", flexShrink: 0 }} />
            ) : (
              <CheckCircle2 size={16} aria-hidden="true" style={{ color: "var(--accent-text)", flexShrink: 0 }} />
            )}
            <span style={{ overflow: "hidden", textOverflow: "ellipsis" }}>{t.message}</span>
          </div>
          <button
            type="button"
            className="btn btn-ghost btn-icon"
            style={{ minWidth: 28, minHeight: 28 }}
            onClick={() => onDismiss(t.id)}
            aria-label="Dismiss notification"
          >
            <X size={14} aria-hidden="true" />
          </button>
        </div>
      ))}
    </div>
  );
}
