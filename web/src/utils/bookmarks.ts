import type { Bookmark, LibrarySection, SmartFilter, SortMode } from "../types";

const STOPWORDS = new Set([
  "the", "and", "for", "with", "that", "this", "from", "into", "your", "about",
  "have", "has", "was", "were", "are", "how", "why", "what", "when", "where",
  "who", "will", "just", "more", "some", "than", "then", "they", "them", "been",
  "http", "https", "www", "com", "org", "net", "post", "status", "thread",
]);

/* Muted stone / paper / salt neutral tones — colour comes from thumbnails, not chrome */
const DOMAIN_PALETTE = [
  { hue: 176, accent: "#E4EFEB" },
  { hue: 48, accent: "#ECEAE2" },
  { hue: 194, accent: "#E6ECEE" },
  { hue: 150, accent: "#E7EEEB" },
  { hue: 38, accent: "#EFECE4" },
  { hue: 200, accent: "#E8ECEE" },
];

export function domainFromURL(raw: string): string {
  try {
    return new URL(raw).hostname.replace(/^www\./, "");
  } catch {
    return raw;
  }
}

export function formatDate(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}

export function formatDateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function isWithinDays(iso: string, days: number): boolean {
  const ts = Date.parse(iso);
  if (Number.isNaN(ts)) return false;
  return Date.now() - ts <= days * 24 * 60 * 60 * 1000;
}

export function getDomainTone(domain: string): { hue: number; accent: string; initial: string } {
  const clean = domain.trim().toLowerCase() || "vault";
  let hash = 0;
  for (let i = 0; i < clean.length; i++) {
    hash = (hash * 31 + clean.charCodeAt(i)) >>> 0;
  }
  const pick = DOMAIN_PALETTE[hash % DOMAIN_PALETTE.length];
  const initialMatch = clean.match(/[a-z0-9]/i);
  const initial = initialMatch ? initialMatch[0].toUpperCase() : "V";
  return { ...pick, initial };
}

export function buildSimilarQuery(bookmark: Bookmark): string {
  const titleWords = bookmark.title
    .toLowerCase()
    .replace(/https?:\/\/\S+/g, " ")
    .replace(/[^a-z0-9\s-]/gi, " ")
    .split(/\s+/)
    .map((w) => w.trim())
    .filter((w) => w.length >= 3 && !STOPWORDS.has(w));

  const unique: string[] = [];
  for (const w of titleWords) {
    if (!unique.includes(w)) unique.push(w);
    if (unique.length >= 2) break;
  }
  if (unique.length > 0) {
    return unique.join(" ");
  }
  if (bookmark.tags.length > 0) {
    return bookmark.tags[0].name;
  }
  const site = (bookmark.site_name || domainFromURL(bookmark.url)).split(".")[0];
  return site && site.length >= 2 ? site : "";
}

export function buildSparklineSeries(bookmarks: Bookmark[], days = 14): number[] {
  const counts = Array.from({ length: days }, () => 0);
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  const todayMs = today.getTime();
  const dayMs = 24 * 60 * 60 * 1000;

  for (const item of bookmarks) {
    const parsed = Date.parse(item.created_at);
    if (Number.isNaN(parsed)) continue;
    const d = new Date(parsed);
    d.setHours(0, 0, 0, 0);
    const diffDays = Math.round((todayMs - d.getTime()) / dayMs);
    if (diffDays >= 0 && diffDays < days) {
      counts[days - 1 - diffDays] += 1;
    }
  }
  return counts;
}

export function filterAndSortBookmarks(
  items: Bookmark[],
  opts: {
    librarySection: LibrarySection;
    smartFilters: SmartFilter[];
    sortMode: SortMode;
    favoriteIds?: Set<number>;
  }
): Bookmark[] {
  const { librarySection, smartFilters, sortMode, favoriteIds } = opts;

  const filtered = items.filter((item) => {
    if (librarySection === "favorites" && !(favoriteIds?.has(item.id) ?? false)) return false;
    if (librarySection === "untagged" && item.tags.length > 0) return false;
    if (librarySection === "recent" && !isWithinDays(item.created_at, 7)) return false;
    if (librarySection === "archived" && !item.archived) return false;

    for (const sf of smartFilters) {
      if (sf === "this_week" && !isWithinDays(item.created_at, 7)) return false;
      if (sf === "with_notes" && item.notes.trim().length === 0) return false;
      if (sf === "has_media" && !item.image_url) return false;
      if (sf === "unlisted" && item.lists.length > 0) return false;
    }
    return true;
  });

  if (sortMode === "newest") {
    return [...filtered].sort((a, b) => Date.parse(b.created_at) - Date.parse(a.created_at) || b.id - a.id);
  }
  if (sortMode === "oldest") {
    return [...filtered].sort((a, b) => Date.parse(a.created_at) - Date.parse(b.created_at) || a.id - b.id);
  }
  return [...filtered].sort((a, b) =>
    (a.title || a.url).localeCompare(b.title || b.url, undefined, { sensitivity: "base" })
  );
}
