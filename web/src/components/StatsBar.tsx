import { useEffect, useMemo, useRef, useState } from "react";
import { Hash } from "lucide-react";
import type { ArchivedFilter, Bookmark, Stats } from "../types";
import { buildSparklineSeries } from "../utils/bookmarks";

interface StatsBarProps {
  stats: Stats | null;
  bookmarks: Bookmark[];
  activeArchivedFilter: ArchivedFilter;
  selectedTag: string;
  onSelectArchived: (filter: ArchivedFilter) => void;
  onSelectTag: (tagName: string) => void;
}

function useCountUp(target: number, durationMs = 420): number {
  const [display, setDisplay] = useState(target);
  const prevRef = useRef(target);

  useEffect(() => {
    const prefersReduced =
      typeof window !== "undefined" &&
      window.matchMedia?.("(prefers-reduced-motion: reduce)").matches;

    if (prefersReduced) {
      prevRef.current = target;
      setDisplay(target);
      return;
    }

    const from = prevRef.current;
    prevRef.current = target;
    if (from === target) {
      setDisplay(target);
      return;
    }

    let raf = 0;
    const start = performance.now();

    function step(now: number) {
      const t = Math.min(1, (now - start) / durationMs);
      const eased = 1 - Math.pow(1 - t, 3);
      setDisplay(Math.round(from + (target - from) * eased));
      if (t < 1) {
        raf = requestAnimationFrame(step);
      }
    }

    raf = requestAnimationFrame(step);
    return () => cancelAnimationFrame(raf);
  }, [target, durationMs]);

  return display;
}

function Sparkline({ series }: { series: number[] }) {
  const width = 92;
  const height = 24;
  const padY = 3;
  const maxVal = Math.max(...series, 1);

  const points = series.map((val, idx) => {
    const x = series.length === 1 ? width / 2 : (idx / (series.length - 1)) * width;
    const y = height - padY - (val / maxVal) * (height - padY * 2);
    return `${x.toFixed(1)},${y.toFixed(1)}`;
  });

  const polyline = points.join(" ");
  const areaPath = `M 0,${height} L ${points.join(" L ")} L ${width},${height} Z`;
  const lastY =
    height - padY - ((series[series.length - 1] ?? 0) / maxVal) * (height - padY * 2);
  const total14d = series.reduce((acc, n) => acc + n, 0);

  return (
    <svg
      className="stats-sparkline"
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      role="img"
      aria-label={`14-day activity sparkline: ${total14d} saved in the last 14 days`}
    >
      <path d={areaPath} className="stats-sparkline-area" />
      <polyline
        fill="none"
        stroke="currentColor"
        strokeWidth="1.75"
        strokeLinecap="round"
        strokeLinejoin="round"
        points={polyline}
      />
      <circle cx={width} cy={lastY} r="2.2" fill="currentColor" />
    </svg>
  );
}

export function StatsBar({
  stats,
  bookmarks,
  activeArchivedFilter,
  selectedTag,
  onSelectArchived,
  onSelectTag,
}: StatsBarProps) {
  const total = stats?.total ?? 0;
  const active = stats?.active ?? 0;
  const archived = stats?.archived ?? 0;
  const topTags = stats?.top_tags?.slice(0, 5) ?? [];

  const animatedTotal = useCountUp(total);
  const sparklineSeries = useMemo(() => buildSparklineSeries(bookmarks, 14), [bookmarks]);

  return (
    <section className="stats-strip" aria-label="Vault summary">
      <div className="stats-hero-block">
        <div className="stats-hero-metric">
          <span className="stats-hero-number tabular-nums">
            {stats ? animatedTotal : "—"}
          </span>
          <span className="stats-hero-label">Saved</span>
        </div>
        <Sparkline series={sparklineSeries} />
      </div>

      <span className="stats-divider" aria-hidden="true" />

      <div className="stats-strip-metrics" role="group" aria-label="Archive filter">
        <button
          type="button"
          className="strip-stat-btn"
          aria-pressed={activeArchivedFilter === "all"}
          onClick={() => onSelectArchived("all")}
        >
          <span className="strip-stat-label">All</span>
          <span className="strip-stat-value tabular-nums">{stats ? total : "—"}</span>
        </button>

        <button
          type="button"
          className="strip-stat-btn"
          aria-pressed={activeArchivedFilter === "false"}
          onClick={() => onSelectArchived("false")}
        >
          <span className="strip-stat-label">Active</span>
          <span className="strip-stat-value tabular-nums">{stats ? active : "—"}</span>
        </button>

        <button
          type="button"
          className="strip-stat-btn"
          aria-pressed={activeArchivedFilter === "true"}
          onClick={() => onSelectArchived("true")}
        >
          <span className="strip-stat-label">Archived</span>
          <span className="strip-stat-value tabular-nums">{stats ? archived : "—"}</span>
        </button>
      </div>

      {topTags.length > 0 && (
        <>
          <span className="stats-divider" aria-hidden="true" />
          <div className="stats-strip-tags" aria-label="Top tags">
            {topTags.map((tag) => {
              const isSelected = selectedTag.toLowerCase() === tag.name.toLowerCase();
              return (
                <button
                  key={tag.id}
                  type="button"
                  className="stat-tag-chip"
                  aria-pressed={isSelected}
                  onClick={() => onSelectTag(tag.name)}
                >
                  <Hash size={11} aria-hidden="true" />
                  <span className="stat-tag-name">{tag.name}</span>
                  <span className="stat-tag-count tabular-nums">{tag.count ?? 0}</span>
                </button>
              );
            })}
          </div>
        </>
      )}
    </section>
  );
}
