export interface Tag {
  id: number;
  name: string;
  count?: number;
  created_at?: string;
}

export interface SmartListRule {
  tags?: string[];
  tag_mode?: "any" | "all";
  query?: string;
  domain?: string;
  archived?: "" | "all" | "false" | "true";
  untagged?: boolean;
}

export interface List {
  id: number;
  name: string;
  description: string;
  is_smart?: boolean;
  rule?: SmartListRule;
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

export interface TagReviewItem {
  id: number;
  bookmark_id: number;
  bookmark_title: string;
  bookmark_url: string;
  bookmark_description?: string;
  bookmark_site_name?: string;
  suggested_tags: string[];
  jev_scores: Record<string, number>;
  source: string;
  status: "pending" | "applied" | "dismissed";
  created_at: string;
  resolved_at?: string;
}

export interface TagReviewListResponse {
  items: TagReviewItem[];
  total: number;
}

export interface BatchTagReviewResult {
  action: "apply" | "dismiss";
  succeeded: number;
  skipped: number;
  failed: number;
  processed_ids: number[];
}

export interface SuggestUntaggedResult {
  processed: number;
  queued: number;
  skipped: number;
  failed: number;
  items: TagReviewItem[];
}

export interface MergeTagsResult {
  merged_into: Tag;
  moved_bookmarks: number;
  deleted_tag_id: number;
}

export interface NsfwStatus {
  configured: boolean;
  locked: boolean;
}

export type ActiveView = "bookmarks" | "tags" | "lists" | "review" | "import";
export type ArchivedFilter = "all" | "false" | "true";
export type LibrarySection = "all" | "favorites" | "untagged" | "recent" | "archived";
export type SmartFilter = "this_week" | "with_notes" | "has_media" | "unlisted";
export type SortMode = "newest" | "oldest" | "title";
export type GridDensity = "comfortable" | "compact";
export type GridStyle = "bento" | "uniform";
