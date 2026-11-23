import { Archive, BookmarkCheck, Hash, Layers } from "lucide-react";
import type { ArchivedFilter, Stats } from "../types";

interface StatsBarProps {
  stats: Stats | null;
  activeArchivedFilter: ArchivedFilter;
  selectedTag: string;
  onSelectArchived: (filter: ArchivedFilter) => void;
  onSelectTag: (tagName: string) => void;
}

export function StatsBar({
  stats,
  activeArchivedFilter,
  selectedTag,
  onSelectArchived,
  onSelectTag,
}: StatsBarProps) {
  const total = stats?.total ?? 0;
  const active = stats?.active ?? 0;
  const archived = stats?.archived ?? 0;
  const topTags = stats?.top_tags?.slice(0, 5) ?? [];

  return (
    <section className="stats-strip glass-rim" aria-label="Vault summary">
      <div className="stats-strip-metrics">
        <button
          type="button"
          className="strip-stat-btn"
          aria-pressed={activeArchivedFilter === "all"}
          onClick={() => onSelectArchived("all")}
        >
          <Layers size={13} aria-hidden="true" />
          <span className="strip-stat-label">All</span>
          <span className="strip-stat-value tabular-nums">{stats ? total : "—"}</span>
        </button>

        <button
          type="button"
          className="strip-stat-btn"
          aria-pressed={activeArchivedFilter === "false"}
          onClick={() => onSelectArchived("false")}
        >
          <BookmarkCheck size={13} aria-hidden="true" />
          <span className="strip-stat-label">Active</span>
          <span className="strip-stat-value tabular-nums">{stats ? active : "—"}</span>
        </button>

        <button
          type="button"
          className="strip-stat-btn"
          aria-pressed={activeArchivedFilter === "true"}
          onClick={() => onSelectArchived("true")}
        >
          <Archive size={13} aria-hidden="true" />
          <span className="strip-stat-label">Archived</span>
          <span className="strip-stat-value tabular-nums">{stats ? archived : "—"}</span>
        </button>
      </div>

      {topTags.length > 0 && (
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
      )}
    </section>
  );
}
