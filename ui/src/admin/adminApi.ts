import type { Category, Item } from "../api";

export interface AdminProfile {
  id: number;
  name: string;
  speaker: string;
  max_volume: number;
}

export interface ProfileInput {
  name: string;
  speaker: string;
  max_volume: number;
}

export interface Room {
  name: string;
  model: string;
}

export interface Preview {
  service: string;
  kind: string;
  title: string;
  cover_url: string;
  artist: string;
  release_date: string;
  /** Where the server would file this link by default (playlists go to "playlist"). */
  category: Category;
}

export interface AddItemsRequest {
  url: string;
  category: Category;
  profile_ids: number[];
  title?: string;
  cover_url?: string;
  artist?: string;
}

/** One release in an artist's discography. */
export interface Album {
  id: string;
  title: string;
  artist: string;
  cover_url: string;
  release_date: string;
  type: "album" | "single" | "compilation" | string;
}

export interface Discography {
  service: string;
  artist: { name: string; image_url: string };
  albums: Album[];
}

export interface BatchItem {
  service: string;
  kind: "album" | "playlist" | "track";
  id: string;
  title: string;
  cover_url: string;
  artist: string;
  release_date: string;
}

export interface BatchResult {
  created: number;
  duplicates: number;
  errors: { profile_id: number; error: string }[];
}

export interface AddItemsResult {
  items: Item[];
  errors: { profile_id: number; error: string }[];
}

/** The token was missing or wrong. */
export class AuthError extends Error {}

/** A failed request; `code` is the server's machine-readable reason, if any. */
export class ApiError extends Error {
  constructor(
    message: string,
    readonly code?: string,
    readonly service?: string,
  ) {
    super(message);
  }
}

async function request<T>(
  token: string,
  method: string,
  path: string,
  body?: unknown,
  allowConflict = false,
): Promise<T> {
  const res = await fetch(`/api/admin${path}`, {
    method,
    headers: {
      Authorization: `Bearer ${token}`,
      ...(body ? { "Content-Type": "application/json" } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 204) return undefined as T;
  const data = (await res.json().catch(() => ({}))) as {
    error?: string;
    code?: string;
    service?: string;
  };
  // 503 normally means "admin disabled"; "not_configured" is a different, feature-level 503.
  if (res.status === 401 || (res.status === 503 && data.code !== "not_configured")) {
    throw new AuthError(data.error ?? res.statusText);
  }
  // Adding items answers 409 with a body listing what was skipped as duplicates.
  if (!res.ok && !(allowConflict && res.status === 409)) {
    throw new ApiError(data.error ?? res.statusText, data.code, data.service);
  }
  return data as T;
}

/** Typed admin endpoints bound to a token. */
export const adminApi = (token: string) => ({
  profiles: () => request<AdminProfile[]>(token, "GET", "/profiles"),
  speakers: () => request<Room[]>(token, "GET", "/speakers"),
  createProfile: (p: ProfileInput) => request<AdminProfile>(token, "POST", "/profiles", p),
  updateProfile: (id: number, p: ProfileInput) =>
    request<AdminProfile>(token, "PUT", `/profiles/${id}`, p),
  deleteProfile: (id: number) => request<void>(token, "DELETE", `/profiles/${id}`),
  library: (id: number) => request<Item[]>(token, "GET", `/profiles/${id}/library`),
  preview: (url: string) => request<Preview>(token, "POST", "/preview", { url }),
  addItems: (r: AddItemsRequest) => request<AddItemsResult>(token, "POST", "/items", r, true),
  discography: (url: string) => request<Discography>(token, "POST", "/discography", { url }),
  addBatch: (r: { category: Category; profile_ids: number[]; items: BatchItem[] }) =>
    request<BatchResult>(token, "POST", "/items/batch", r, true),
  updateItem: (
    id: number,
    patch: {
      title?: string;
      cover_url?: string;
      category?: Category;
      artist?: string;
      release_date?: string;
      /** Mark as played (true) or unplayed (false). */
      played?: boolean;
    },
  ) => request<Item>(token, "PUT", `/items/${id}`, patch),
  deleteItem: (id: number) => request<void>(token, "DELETE", `/items/${id}`),
  /** Looks up missing artists, dates and covers for everything saved. */
  refreshMetadata: () =>
    request<{ checked: number; updated: number }>(token, "POST", "/items/refresh"),
});

export type AdminApi = ReturnType<typeof adminApi>;
