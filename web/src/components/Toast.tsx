import { AlertCircle, CheckCircle2, X } from "lucide-react";

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
        <div key={t.id} className="toast glass-rim" data-tone={t.tone}>
          <div className="toast-content">
            {t.tone === "error" ? (
              <AlertCircle size={16} aria-hidden="true" className="toast-icon toast-icon-error" />
            ) : (
              <CheckCircle2 size={16} aria-hidden="true" className="toast-icon toast-icon-success" />
            )}
            <span className="toast-message">{t.message}</span>
          </div>
          <button
            type="button"
            className="btn btn-ghost btn-icon toast-close-btn"
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
