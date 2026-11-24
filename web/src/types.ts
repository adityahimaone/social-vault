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

export type ActiveView = "bookmarks" | "tags" | "lists" | "import";
export type ArchivedFilter = "all" | "false" | "true";
export type LibrarySection = "all" | "favorites" | "untagged" | "recent" | "archived";
export type SmartFilter = "this_week" | "with_notes" | "has_media" | "unlisted";
export type SortMode = "newest" | "oldest" | "title";
export type GridDensity = "comfortable" | "compact";
export type GridStyle = "bento" | "uniform";
