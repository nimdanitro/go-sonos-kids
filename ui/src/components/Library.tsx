import { useEffect, useState, type ReactNode } from "react";
import { api, type Category, type Item, type Profile, type Status } from "../api";
import { accentFor } from "../hooks";
import { groupByArtist } from "../lib/group";
import { strings } from "../strings";
import { Cover, Initial } from "./Cover";
import { ProgressBar } from "./ProgressBar";
import {
  BackIcon,
  BookIcon,
  CheckIcon,
  ListIcon,
  NoteIcon,
  PauseIcon,
  PlayIcon,
  Spinner,
} from "./icons";

interface Props {
  profile: Profile;
  startingId: number | null;
  status: Status | null;
  onPlay: (item: Item) => void;
  onOpenPlayer: () => void;
  onTogglePlay: () => void;
  onSwitch: () => void;
  /** Where in the library the kid is. Kept by the parent so that coming back from the player restores it. */
  tab: Category;
  artistKey: string | null;
  onNavigate: (tab: Category, artistKey: string | null) => void;
}

const TABS = [
  ["music", strings.music, NoteIcon],
  ["audiobook", strings.audiobooks, BookIcon],
  ["playlist", strings.playlists, ListIcon],
] as const;

const titleClass =
  "line-clamp-3 text-center text-[clamp(1rem,2.2vmin,1.35rem)] leading-tight font-extrabold wrap-break-word text-slate-100";

/** One cover with its name underneath; artists get a round cover. */
function Tile({
  title,
  subtitle,
  cover,
  round = false,
  busy = false,
  played = false,
  progress,
  disabled = false,
  onClick,
}: {
  title: string;
  subtitle?: ReactNode;
  cover: string;
  round?: boolean;
  busy?: boolean;
  /** Already heard: shown with a check mark and a dimmed cover. */
  played?: boolean;
  /** Partly heard, 0..1: shown as a thin bar along the bottom of the cover. */
  progress?: number;
  disabled?: boolean;
  onClick: () => void;
}) {
  // Badges sit in the top-right corner; a round cover needs them moved inwards.
  const corner = round ? "top-[15%] right-[15%]" : "top-2 right-2";
  return (
    <li>
      <button
        onClick={onClick}
        disabled={disabled}
        className="group flex w-full animate-pop flex-col gap-2 text-left transition active:scale-95"
      >
        <div
          className={`relative aspect-square w-full overflow-hidden bg-slate-800 shadow-lg ring-4 shadow-black/50 ring-slate-700 ${
            round ? "rounded-full" : "rounded-3xl"
          }`}
        >
          <Cover url={cover} title={title} className="size-full" />
          {played && (
            <>
              <div className="absolute inset-0 bg-slate-950/50" />
              <span
                className={`absolute ${corner} flex size-9 items-center justify-center rounded-full bg-emerald-500 text-white shadow-lg ring-2 ring-slate-950/40`}
              >
                <CheckIcon className="size-6" />
                <span className="sr-only">{strings.played}</span>
              </span>
            </>
          )}
          {progress !== undefined && <ProgressBar fraction={progress} />}
          {busy && (
            <div className="absolute inset-0 flex items-center justify-center bg-black/50 text-white">
              <Spinner className="size-12" />
            </div>
          )}
        </div>
        <span className={titleClass}>{title}</span>
        {subtitle && (
          <span className="-mt-1 block text-center text-sm font-bold text-slate-400">
            {subtitle}
          </span>
        )}
      </button>
    </li>
  );
}

/**
 * Heard to the end, as opposed to just started. Audiobooks played before
 * progress was tracked have no progress and count as heard.
 */
const heard = (item: Item) => item.played && (!item.progress || item.progress.completed);

/** Started but not finished. */
const inProgress = (item: Item) => !!item.progress && !item.progress.completed;

/**
 * "5 Folgen · 2 gehört" for a series, with what is in progress on its own
 * accent-coloured line; "3 Alben" for an artist.
 */
function seriesSubtitle(tab: Category, items: Item[]): ReactNode {
  if (tab !== "audiobook") return strings.albumCount(items.length);
  const played = items.filter(heard).length;
  const started = items.filter(inProgress).length;
  const base =
    played > 0
      ? `${strings.episodeCount(items.length)} · ${strings.playedCount(played)}`
      : strings.episodeCount(items.length);
  if (started === 0) return base;
  return (
    <>
      {base}
      <span className="block text-(--accent)">{strings.startedCount(started)}</span>
    </>
  );
}

const gridClass =
  "grid grid-cols-[repeat(auto-fill,minmax(clamp(9.5rem,24vmin,16rem),1fr))] gap-x-4 gap-y-5";

export function Library({
  profile,
  startingId,
  status,
  onPlay,
  onOpenPlayer,
  onTogglePlay,
  onSwitch,
  tab,
  artistKey,
  onNavigate,
}: Props) {
  const [items, setItems] = useState<Item[] | null>(null);

  useEffect(() => {
    api
      .library(profile.id)
      .then(setItems)
      .catch(() => setItems([]));
  }, [profile.id]);

  const inCategory = (c: Category) => (items ?? []).filter((i) => i.category === c);
  // Music is always offered; the other tabs only when there is something in them.
  const tabs = TABS.filter(([key]) => key === "music" || inCategory(key).length > 0);
  const activeTab: Category = tabs.some(([key]) => key === tab) ? tab : "music";

  const shown = inCategory(activeTab);
  // Music and audiobooks are organised by artist (or series) first, then by album or episode.
  const grouped = activeTab === "music" || activeTab === "audiobook";
  const artists = grouped ? groupByArtist(shown) : [];
  const openArtist = artistKey === null ? undefined : artists.find((g) => g.key === artistKey);
  const hasPlayer =
    status && status.title && (status.state === "PLAYING" || status.state === "PAUSED_PLAYBACK");

  const itemTiles = (list: Item[]) =>
    list.map((item) => (
      <Tile
        key={item.id}
        title={item.title}
        cover={item.cover_url}
        played={item.category === "audiobook" && heard(item)}
        progress={
          item.category === "audiobook" && item.progress && !item.progress.completed
            ? item.progress.fraction
            : undefined
        }
        subtitle={
          item.category === "audiobook" && item.progress && !item.progress.completed
            ? strings.resume(item.progress.track, item.progress.total)
            : undefined
        }
        busy={startingId === item.id}
        disabled={startingId !== null}
        onClick={() => onPlay(item)}
      />
    ));

  return (
    <main
      style={{ ["--accent" as string]: accentFor(profile.id) }}
      className="flex h-full flex-col bg-linear-to-br from-slate-950 via-slate-900 to-indigo-950"
    >
      <header className={`flex items-center gap-3 px-4 pt-3 ${openArtist ? "pb-5" : "pb-2"}`}>
        {openArtist ? (
          <>
            <button
              onClick={() => onNavigate(activeTab, null)}
              aria-label={strings.back}
              className="flex h-14 shrink-0 items-center gap-1 rounded-full bg-slate-800/90 pr-5 pl-3 text-xl font-extrabold text-slate-100 shadow-lg transition active:scale-95"
            >
              <BackIcon className="size-7" />
              {strings.back}
            </button>
            <h1 className="min-w-0 truncate text-[clamp(1.4rem,4vmin,2.2rem)] font-extrabold text-slate-50">
              {openArtist.name || strings.unknownArtist}
            </h1>
          </>
        ) : (
          <>
            <button
              onClick={onSwitch}
              aria-label={`${strings.switchProfile} (${profile.name})`}
              className="flex size-14 shrink-0 items-center justify-center rounded-full border-4 border-(--accent) bg-slate-800 text-3xl text-(--accent) shadow transition active:scale-95"
            >
              <Initial name={profile.name} />
            </button>

            {tabs.length > 1 && (
              <nav
                className="ml-auto flex gap-2 rounded-full bg-slate-800/70 p-1.5 shadow-inner"
                aria-label={strings.category}
              >
                {tabs.map(([key, label, Icon]) => (
                  <button
                    key={key}
                    onClick={() => {
                      onNavigate(key, null);
                    }}
                    aria-pressed={activeTab === key}
                    aria-label={label}
                    className={`flex h-12 items-center gap-2 rounded-full px-5 text-lg font-extrabold transition active:scale-95 max-[520px]:px-4 ${
                      activeTab === key ? "bg-(--accent) text-white shadow" : "text-slate-300"
                    }`}
                  >
                    <Icon className="size-6" />
                    <span className="max-[520px]:hidden">{label}</span>
                  </button>
                ))}
              </nav>
            )}
          </>
        )}
      </header>

      <section
        className={`min-h-0 flex-1 overflow-y-auto px-4 pt-2 ${hasPlayer ? "pb-28" : "pb-6"}`}
      >
        {items === null ? (
          <p className="mt-16 text-center text-2xl font-bold text-slate-400">{strings.loading}</p>
        ) : shown.length === 0 ? (
          <p className="mx-auto mt-16 max-w-md text-center text-2xl font-bold text-slate-400">
            {strings.emptyLibrary}
          </p>
        ) : (
          <ul className={gridClass}>
            {openArtist
              ? itemTiles(openArtist.items)
              : grouped
                ? artists.map((g) => (
                    <Tile
                      key={g.key}
                      round
                      title={g.name || strings.unknownArtist}
                      subtitle={seriesSubtitle(activeTab, g.items)}
                      played={activeTab === "audiobook" && g.items.every(heard)}
                      cover={g.cover}
                      onClick={() => onNavigate(activeTab, g.key)}
                    />
                  ))
                : itemTiles(shown)}
          </ul>
        )}
      </section>

      {hasPlayer && (
        <div className="absolute inset-x-3 bottom-3 flex animate-pop items-center gap-3 rounded-3xl bg-slate-800 p-2 pr-3 shadow-2xl ring-4 shadow-black/60 ring-(--accent)">
          <button
            onClick={onOpenPlayer}
            className="flex min-w-0 flex-1 items-center gap-3 text-left active:opacity-70"
          >
            <Cover
              url={status.cover_url}
              title={status.title}
              className="size-16 shrink-0 rounded-2xl text-3xl"
            />
            <span className="min-w-0">
              <span className="block truncate text-xl font-extrabold text-slate-50">
                {status.title}
              </span>
              <span className="block truncate text-base font-semibold text-slate-400">
                {status.artist || status.album}
              </span>
            </span>
          </button>
          <button
            onClick={onTogglePlay}
            aria-label={status.state === "PLAYING" ? strings.pause : strings.play}
            className="flex size-16 shrink-0 items-center justify-center rounded-full bg-(--accent) text-white shadow transition active:scale-90"
          >
            {status.state === "PLAYING" ? (
              <PauseIcon className="size-9" />
            ) : (
              <PlayIcon className="size-9" />
            )}
          </button>
        </div>
      )}
    </main>
  );
}
