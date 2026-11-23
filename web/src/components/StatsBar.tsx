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
  const activeRatio = total > 0 ? Math.round((active / total) * 100) : 0;
  const topTags = stats?.top_tags?.slice(0, 4) ?? [];

  return (
    <section className="stats-bento" aria-label="Vault statistics">
      <button
        type="button"
        className="stat-card stat-card-hero stat-card-btn glass-rim"
        aria-pressed={activeArchivedFilter === "all"}
        onClick={() => onSelectArchived("all")}
      >
        <div className="stat-card-top">
          <span className="stat-label">Vault Total</span>
          <span className="stat-icon-well" aria-hidden="true">
            <Layers size={15} />
          </span>
        </div>
        <span className="stat-value">{stats ? total : "—"}</span>
        <div className="stat-meter" aria-hidden="true">
          <span
            className="stat-meter-fill"
            style={{ width: `${activeRatio}%` }}
          />
        </div>
        <span className="stat-sub tabular-nums">
          {stats ? `${active} active · ${stats.lists_count} lists` : "Loading vault..."}
        </span>
      </button>

      <button
        type="button"
        className="stat-card stat-card-btn glass-rim"
        aria-pressed={activeArchivedFilter === "false"}
        onClick={() => onSelectArchived("false")}
      >
        <div className="stat-card-top">
          <span className="stat-label">Active</span>
          <span className="stat-icon-well" aria-hidden="true">
            <BookmarkCheck size={15} />
          </span>
        </div>
        <span className="stat-value">{stats ? active : "—"}</span>
        <span className="stat-sub tabular-nums">
          {stats ? `${activeRatio}% of archive` : "Loading..."}
        </span>
      </button>

      <button
        type="button"
        className="stat-card stat-card-btn glass-rim"
        aria-pressed={activeArchivedFilter === "true"}
        onClick={() => onSelectArchived("true")}
      >
        <div className="stat-card-top">
          <span className="stat-label">Archived</span>
          <span className="stat-icon-well" aria-hidden="true">
            <Archive size={15} />
          </span>
        </div>
        <span className="stat-value">{stats ? archived : "—"}</span>
        <span className="stat-sub tabular-nums">
          {stats ? `${stats.lists_count} curated lists` : "Loading..."}
        </span>
      </button>

      <div className="stat-card stat-card-tags glass-rim">
        <div className="stat-card-top">
          <span className="stat-label">Top Tags</span>
          <span className="stat-sub tabular-nums">
            {stats ? `${stats.tags_count} total` : "—"}
          </span>
        </div>
        {topTags.length > 0 ? (
          <div className="stat-tags-row">
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
        ) : (
          <div className="stat-tags-empty">
            <span className="stat-sub">No tags yet — tag links as you save</span>
          </div>
        )}
      </div>
    </section>
  );
}
