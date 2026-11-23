import { useCallback, useLayoutEffect, useRef, useState, type ReactNode } from "react";

export interface GlassSegmentedOption<T extends string> {
  value: T;
  label: ReactNode;
  ariaLabel?: string;
}

interface GlassSegmentedProps<T extends string> {
  options: readonly GlassSegmentedOption<T>[];
  value: T;
  onChange: (value: T) => void;
  ariaLabel: string;
  size?: "sm" | "md";
  className?: string;
}

export function GlassSegmented<T extends string>({
  options,
  value,
  onChange,
  ariaLabel,
  size = "md",
  className = "",
}: GlassSegmentedProps<T>) {
  const trackRef = useRef<HTMLDivElement | null>(null);
  const lensRef = useRef<HTMLSpanElement | null>(null);
  const btnRefs = useRef<Record<string, HTMLButtonElement | null>>({});
  const [ready, setReady] = useState(false);

  const syncLens = useCallback(() => {
    const activeBtn = btnRefs.current[value];
    const lens = lensRef.current;
    if (!activeBtn || !lens) return;
    lens.style.width = `${activeBtn.offsetWidth}px`;
    lens.style.transform = `translateX(${activeBtn.offsetLeft - 3}px)`;
    setReady(true);
  }, [value]);

  useLayoutEffect(() => {
    syncLens();
    const track = trackRef.current;
    if (!track || typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(() => syncLens());
    ro.observe(track);
    return () => ro.disconnect();
  }, [syncLens, options.length]);

  return (
    <div
      ref={trackRef}
      className={`glass-segmented glass-segmented-${size} ${className}`.trim()}
      role="group"
      aria-label={ariaLabel}
    >
      <span
        ref={lensRef}
        className={`glass-lens ${ready ? "is-ready" : ""}`}
        aria-hidden="true"
      />
      {options.map((opt) => {
        const active = opt.value === value;
        return (
          <button
            key={opt.value}
            ref={(el) => {
              btnRefs.current[opt.value] = el;
            }}
            type="button"
            className="glass-segmented-btn"
            aria-pressed={active}
            aria-label={opt.ariaLabel}
            onClick={() => onChange(opt.value)}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}
