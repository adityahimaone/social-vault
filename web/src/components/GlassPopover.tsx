import { useEffect, useRef, useState, type ReactNode } from "react";

interface GlassPopoverProps {
  open: boolean;
  onClose: () => void;
  trigger: ReactNode;
  children: ReactNode;
  align?: "left" | "right";
  label: string;
  className?: string;
}

export function GlassPopover({
  open,
  onClose,
  trigger,
  children,
  align = "right",
  label,
  className = "",
}: GlassPopoverProps) {
  const wrapRef = useRef<HTMLDivElement | null>(null);
  const [rendered, setRendered] = useState(open);
  const [state, setState] = useState<"entering" | "open" | "closing">(open ? "open" : "closing");

  useEffect(() => {
    if (open) {
      setRendered(true);
      setState("entering");
      const raf = requestAnimationFrame(() => setState("open"));
      return () => cancelAnimationFrame(raf);
    }
    if (!rendered) return;
    setState("closing");
    const timer = window.setTimeout(() => setRendered(false), 135);
    return () => window.clearTimeout(timer);
  }, [open, rendered]);

  useEffect(() => {
    if (!open) return;

    function onPointerDown(e: MouseEvent) {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) {
        onClose();
      }
    }

    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") {
        e.preventDefault();
        e.stopPropagation();
        setRendered(false);
        onClose();
      }
    }

    document.addEventListener("mousedown", onPointerDown);
    window.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      window.removeEventListener("keydown", onKeyDown);
    };
  }, [open, onClose]);

  return (
    <div ref={wrapRef} className={`popover-anchor ${className}`.trim()}>
      {trigger}
      {rendered && (
        <div
          className={`nav-popover nav-popover-${align}`}
          data-state={state}
          role="dialog"
          aria-label={label}
        >
          {children}
        </div>
      )}
    </div>
  );
}
