import type { Item } from "../api";

export interface ArtistGroup {
  /** Stable key: the artist name, case-insensitive. Empty for "unknown". */
  key: string;
  /** Name to show; empty when the artist is unknown. */
  name: string;
  /** This artist's releases, in the order given (the server sorts oldest first). */
  items: Item[];
  /** Cover for the artist tile: the first release that has one. */
  cover: string;
}

/** Groups music by artist, keeping the server's order (artists A–Z, unknown last). */
export function groupByArtist(items: Item[]): ArtistGroup[] {
  const groups = new Map<string, ArtistGroup>();
  for (const item of items) {
    const name = item.artist.trim();
    const key = name.toLowerCase();
    let group = groups.get(key);
    if (!group) {
      group = { key, name, items: [], cover: "" };
      groups.set(key, group);
    }
    group.items.push(item);
    group.cover ||= item.cover_url;
  }
  return [...groups.values()];
}
