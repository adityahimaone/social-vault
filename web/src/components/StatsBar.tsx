import type { ArchivedFilter, Stats } from "../types";

interface StatsBarProps {
  stats: Stats | null;
  onSelectArchived: (filter: ArchivedFilter) => void;
  onSelectTag: (tagName: string) => void;
}

export function StatsBar({ stats, onSelectArchived, onSelectTag }: StatsBarProps) {
  const topTag = stats?.top_tags?.[0];

  return (
    <section className="stats-grid" aria-label="Vault statistics">
      <button
        type="button"
        className="stat-card stat-card-btn"
        onClick={() => onSelectArchived("all")}
      >
        <span className="stat-label">Total Bookmarks</span>
        <span className="stat-value">{stats ? stats.total : "—"}</span>
        <span className="stat-sub tabular-nums">
          {stats ? `${stats.active} active` : "Loading..."}
        </span>
      </button>

      <button
        type="button"
        className="stat-card stat-card-btn"
        onClick={() => onSelectArchived("true")}
      >
        <span className="stat-label">Archived</span>
        <span className="stat-value">{stats ? stats.archived : "—"}</span>
        <span className="stat-sub tabular-nums">
          {stats ? `${stats.lists_count} lists` : "Loading..."}
        </span>
      </button>

      {topTag ? (
        <button
          type="button"
          className="stat-card stat-card-btn"
          onClick={() => onSelectTag(topTag.name)}
        >
          <span className="stat-label">Top Tag</span>
          <span className="stat-value" style={{ fontSize: 20, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
            #{topTag.name}
          </span>
          <span className="stat-sub tabular-nums">
            {topTag.count ?? 0} links · {stats?.tags_count ?? 0} tags total
          </span>
        </button>
      ) : (
        <div className="stat-card">
          <span className="stat-label">Top Tag</span>
          <span className="stat-value" style={{ fontSize: 20 }}>None yet</span>
          <span className="stat-sub tabular-nums">{stats?.tags_count ?? 0} tags total</span>
        </div>
      )}
    </section>
  );
}
