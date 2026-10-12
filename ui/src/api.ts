export interface Profile {
  id: number;
  name: string;
  max_volume: number;
  avatar: string;
}

export type Category = "music" | "audiobook" | "playlist";

export interface Progress {
  /** Track last heard, and how many the album has (0 if unknown). */
  track: number;
  total: number;
  /** Seconds into that track. */
  position: number;
  title: string;
  completed: boolean;
  /** 0..1 through the whole audiobook. */
  fraction: number;
}

export interface Item {
  id: number;
  category: Category;
  title: string;
  cover_url: string;
  /** Who made it; music is grouped by this. Empty when unknown. */
  artist: string;
  /** YYYY-MM-DD, or empty when unknown. The server already sorts by it. */
  release_date: string;
  /** Whether this kid has started it before (shown for audiobooks). */
  played: boolean;
  /** How far this kid got in an audiobook, if it was started and tracked. */
  progress?: Progress;
  service: string;
  kind: string;
}

export interface Status {
  state: "PLAYING" | "PAUSED_PLAYBACK" | "STOPPED" | "TRANSITIONING" | string;
  title: string;
  artist: string;
  album: string;
  cover_url: string;
  volume: number;
  max_volume: number;
  /** How far through the whole album or queue, 0..1; absent for streams. */
  fraction?: number;
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error((data as { error?: string }).error ?? res.statusText);
  return data as T;
}

const p = (id: number) => `/api/profiles/${id}`;

export const api = {
  profiles: () => request<Profile[]>("GET", "/api/profiles"),
  library: (id: number) => request<Item[]>("GET", `${p(id)}/library`),
  status: (id: number) => request<Status>("GET", `${p(id)}/status`),
  /** Starts an item; audiobooks resume where they were left unless `restart` is set. */
  play: (id: number, itemId: number, restart = false) =>
    request("POST", `${p(id)}/play`, { item_id: itemId, restart }),
  pause: (id: number) => request("POST", `${p(id)}/pause`),
  resume: (id: number) => request("POST", `${p(id)}/resume`),
  next: (id: number) => request("POST", `${p(id)}/next`),
  previous: (id: number) => request("POST", `${p(id)}/previous`),
  volume: (id: number, level: number) => request("POST", `${p(id)}/volume`, { level }),
};
