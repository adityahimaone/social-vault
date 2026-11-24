export interface Tag {
  id: number;
  name: string;
  count?: number;
  created_at?: string;
}

export interface List {
  id: number;
  name: string;
  description: string;
  count?: number;
  created_at?: string;
}

export interface Bookmark {
  id: number;
  url: string;
  title: string;
  description: string;
  notes: string;
  favicon_url: string;
  image_url?: string;
  site_name: string;
  source: string;
  archived: boolean;
  created_at: string;
  updated_at: string;
  tags: Tag[];
  lists: List[];
}

export interface Stats {
  total: number;
  active: number;
  archived: number;
  tags_count: number;
  lists_count: number;
  top_tags: Tag[];
}

export interface ImportResult {
  format: string;
  total: number;
  imported: number;
  skipped: number;
  errors: number;
}

export interface ActiveJevJob {
  bookmark_id: number;
  started_at: string;
}

export interface JevEvent {
  seq: number;
  bookmark_id: number;
  applied_tags: string[];
  queued: boolean;
  error: string;
  finished_at: string;
}

export interface BackfillResult {
  processed: number;
  rules_tagged: number;
  jev_tagged: number;
  queued: number;
  skipped: number;
  failed: number;
}

export interface BackfillStatus {
  running: boolean;
  started_at: string;
  finished_at: string;
  result: BackfillResult | null;
}

export interface TaggingStatus {
  jev_enabled: boolean;
  active_jev_jobs: ActiveJevJob[];
  last_jev_event: JevEvent | null;
  last_backfill: BackfillStatus;
  pending_review: number;
  untagged_count: number;
}

export interface NsfwStatus {
  configured: boolean;
  locked: boolean;
}

export type ActiveView = "bookmarks" | "tags" | "lists" | "import";
export type ArchivedFilter = "all" | "false" | "true";
export type LibrarySection = "all" | "favorites" | "untagged" | "recent" | "archived";
export type SmartFilter = "this_week" | "with_notes" | "has_media" | "unlisted";
export type SortMode = "newest" | "oldest" | "title";
export type GridDensity = "comfortable" | "compact";
export type GridStyle = "bento" | "uniform";
