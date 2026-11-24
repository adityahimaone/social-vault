import type { Bookmark, ImportResult, List, Stats, Tag, TaggingStatus } from "./types";

export type { TaggingStatus } from "./types";

let onUnauthorizedHandler: (() => void) | null = null;

export function setUnauthorizedHandler(fn: () => void) {
  onUnauthorizedHandler = fn;
}

export class ApiError extends Error {
  status: number;
  data?: unknown;
  constructor(status: number, message: string, data?: unknown) {
    super(message);
    this.status = status;
    this.data = data;
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !(init.body instanceof FormData) && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(path, {
    ...init,
    headers,
    credentials: "same-origin",
  });

  if (res.status === 401 && path !== "/api/auth/login" && path !== "/api/auth/me") {
    onUnauthorizedHandler?.();
  }

  const text = await res.text();
  let parsed: unknown = null;
  if (text) {
    try {
      parsed = JSON.parse(text);
    } catch {
      parsed = null;
    }
  }

  if (!res.ok) {
    const msg =
      parsed && typeof parsed === "object" && "error" in parsed && typeof (parsed as { error?: unknown }).error === "string"
        ? (parsed as { error: string }).error
        : `Request failed (${res.status})`;
    throw new ApiError(res.status, msg, parsed);
  }

  return parsed as T;
}

export const api = {
  authMe: () => request<{ authenticated: boolean; password_configured: boolean }>("/api/auth/me"),

  login: (password: string) =>
    request<{ authenticated: boolean }>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ password }),
    }),

  logout: () =>
    request<{ authenticated: boolean }>("/api/auth/logout", {
      method: "POST",
    }),

  stats: () => request<Stats>("/api/stats"),

  listBookmarks: (
    params: { q?: string; archived?: string; tag?: string; list?: string; limit?: number; offset?: number },
    signal?: AbortSignal
  ) => {
    const sp = new URLSearchParams();
    if (params.q) sp.set("q", params.q);
    if (params.archived && params.archived !== "all") sp.set("archived", params.archived);
    if (params.tag) sp.set("tag", params.tag);
    if (params.list) sp.set("list", params.list);
    if (params.limit) sp.set("limit", String(params.limit));
    if (params.offset) sp.set("offset", String(params.offset));
    const qs = sp.toString();
    return request<{ items: Bookmark[]; total: number; limit: number; offset: number }>(
      `/api/bookmarks${qs ? `?${qs}` : ""}`,
      { signal }
    );
  },

  createBookmark: (payload: {
    url: string;
    title?: string;
    notes?: string;
    tags?: string[];
    lists?: string[];
  }) =>
    request<Bookmark>("/api/bookmarks", {
      method: "POST",
      body: JSON.stringify(payload),
    }),

  updateBookmark: (
    id: number,
    payload: {
      title?: string;
      description?: string;
      notes?: string;
      archived?: boolean;
      tags?: string[];
      list_ids?: number[];
    }
  ) =>
    request<Bookmark>(`/api/bookmarks/${id}`, {
      method: "PATCH",
      body: JSON.stringify(payload),
    }),

  deleteBookmark: (id: number) =>
    request<{ deleted: boolean }>(`/api/bookmarks/${id}`, {
      method: "DELETE",
    }),

  listTags: () => request<{ items: Tag[] }>("/api/tags"),

  createTag: (name: string) =>
    request<Tag>("/api/tags", {
      method: "POST",
      body: JSON.stringify({ name }),
    }),

  deleteTag: (id: number) =>
    request<{ deleted: boolean }>(`/api/tags/${id}`, {
      method: "DELETE",
    }),

  listLists: () => request<{ items: List[] }>("/api/lists"),

  createList: (name: string, description: string) =>
    request<List>("/api/lists", {
      method: "POST",
      body: JSON.stringify({ name, description }),
    }),

  deleteList: (id: number) =>
    request<{ deleted: boolean }>(`/api/lists/${id}`, {
      method: "DELETE",
    }),

  importFile: (file: File, format: "auto" | "netscape" | "karakeep") => {
    const fd = new FormData();
    fd.append("file", file);
    fd.append("format", format);
    return request<ImportResult>("/api/import", {
      method: "POST",
      body: fd,
    });
  },

  taggingStatus: (signal?: AbortSignal) =>
    request<TaggingStatus>("/api/tagging/status", { signal }),
};
