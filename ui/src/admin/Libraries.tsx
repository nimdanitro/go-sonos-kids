import { useEffect, useRef, useState } from "react";
import type { Category, Item } from "../api";
import { Cover } from "../components/Cover";
import { admin } from "../strings";
import type { AdminApi, AdminProfile } from "./adminApi";
import { Button, Card, Field, Select, TextInput } from "./ui";

interface Props {
  api: AdminApi;
  profiles: AdminProfile[];
  libraries: Record<number, Item[]>;
  notify: (text: string, ok?: boolean) => void;
  run: <T>(fn: () => Promise<T>) => Promise<T | undefined>;
  onChanged: () => void;
}

const CATEGORIES: [Category, string][] = [
  ["music", admin.categoryMusic],
  ["audiobook", admin.categoryAudiobook],
  ["playlist", admin.categoryPlaylist],
];

function ItemRow({
  item,
  api,
  run,
  onChanged,
}: { item: Item } & Pick<Props, "api" | "run" | "onChanged">) {
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState(item.title);
  const [artist, setArtist] = useState(item.artist);
  const [date, setDate] = useState(item.release_date);
  const titleInput = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (editing) titleInput.current?.focus();
  }, [editing]);

  async function update(patch: Parameters<AdminApi["updateItem"]>[1]) {
    if (await run(() => api.updateItem(item.id, patch))) {
      setEditing(false);
      onChanged();
    }
  }

  const meta = [item.artist, item.release_date.slice(0, 4), `${item.service} ${item.kind}`]
    .filter(Boolean)
    .join(" · ");
  // Music is grouped by artist, so one without it ends up under "Verschiedene".
  const missingArtist = item.category === "music" && item.kind !== "playlist" && !item.artist;
  const isAudiobook = item.category === "audiobook";

  return (
    <li className="flex items-center gap-3 border-b border-slate-700/70 py-2 last:border-b-0">
      <Cover
        url={item.cover_url}
        title={item.title}
        className="size-14 shrink-0 rounded-xl text-2xl"
      />
      <div className="min-w-0 flex-1">
        {editing ? (
          <form
            className="grid gap-2 sm:grid-cols-[2fr_1.4fr_1fr_auto_auto]"
            onSubmit={(e) => {
              e.preventDefault();
              void update({ title, artist, release_date: date });
            }}
          >
            <Field label={admin.name}>
              <TextInput
                ref={titleInput}
                required
                value={title}
                onChange={(e) => setTitle(e.target.value)}
              />
            </Field>
            <Field label={admin.artistLabel}>
              <TextInput value={artist} onChange={(e) => setArtist(e.target.value)} />
            </Field>
            <Field label={admin.dateLabel}>
              <TextInput
                value={date}
                placeholder="2015-06-01"
                onChange={(e) => setDate(e.target.value)}
              />
            </Field>
            <Button type="submit" tone="primary" className="self-end">
              {admin.save}
            </Button>
            <Button type="button" className="self-end" onClick={() => setEditing(false)}>
              {admin.cancel}
            </Button>
          </form>
        ) : (
          <>
            <p className="truncate font-extrabold text-slate-50">{item.title}</p>
            <p className="truncate text-sm font-bold text-slate-400">
              {missingArtist && (
                <span className="mr-2 text-amber-400">{admin.missingArtist} ·</span>
              )}
              {isAudiobook && item.progress && !item.progress.completed && (
                <span className="mr-2 text-sky-400">
                  ▶{" "}
                  {admin.progressTag(
                    item.progress.track,
                    item.progress.total,
                    item.progress.position,
                  )}{" "}
                  ·
                </span>
              )}
              {isAudiobook && item.progress?.completed && (
                <span className="mr-2 text-emerald-400">✓ {admin.finishedTag} ·</span>
              )}
              {isAudiobook && item.played && !item.progress && (
                <span className="mr-2 text-emerald-400">✓ {admin.playedTag} ·</span>
              )}
              {meta}
            </p>
          </>
        )}
      </div>
      {!editing && (
        <div className="flex flex-wrap items-center justify-end gap-2">
          <Button onClick={() => setEditing(true)}>{admin.edit}</Button>
          {isAudiobook && (
            <Button onClick={() => void update({ played: !item.played })}>
              {item.played ? admin.markUnplayed : admin.markPlayed}
            </Button>
          )}
          <Select
            aria-label={admin.moveTo}
            value=""
            className="w-44!"
            onChange={(e) => void update({ category: e.target.value as Category })}
          >
            <option value="" disabled>
              {admin.moveTo}…
            </option>
            {CATEGORIES.filter(([key]) => key !== item.category).map(([key, label]) => (
              <option key={key} value={key}>
                {label}
              </option>
            ))}
          </Select>
          <Button
            tone="danger"
            onClick={async () => {
              await run(() => api.deleteItem(item.id));
              onChanged();
            }}
          >
            {admin.remove}
          </Button>
        </div>
      )}
    </li>
  );
}

export function Libraries({ api, profiles, libraries, notify, run, onChanged }: Props) {
  const [refreshing, setRefreshing] = useState(false);

  async function refresh() {
    setRefreshing(true);
    const res = await run(() => api.refreshMetadata());
    setRefreshing(false);
    if (res) {
      notify(admin.refreshed(res.updated, res.checked));
      onChanged();
    }
  }

  return (
    <>
      <div className="flex flex-wrap items-center gap-3 rounded-2xl bg-slate-800/50 p-3">
        <Button onClick={refresh} disabled={refreshing}>
          {refreshing ? admin.refreshing : admin.refreshButton}
        </Button>
        <p className="min-w-0 flex-1 text-sm font-bold text-slate-400">{admin.refreshHint}</p>
      </div>
      {profiles.map((p) => {
        const items = libraries[p.id] ?? [];
        return (
          <Card key={p.id} title={admin.libraryTitle(p.name)}>
            {items.length === 0 ? (
              <p className="font-bold text-slate-400">{admin.emptyLibrary}</p>
            ) : (
              CATEGORIES.map(([cat, label]) => {
                const inCat = items.filter((i) => i.category === cat);
                if (inCat.length === 0) return null;
                return (
                  <div key={cat} className="mb-3 last:mb-0">
                    <h3 className="mb-1 text-sm font-extrabold tracking-wide text-sky-400 uppercase">
                      {label} ({inCat.length})
                    </h3>
                    <ul>
                      {inCat.map((it) => (
                        <ItemRow key={it.id} item={it} api={api} run={run} onChanged={onChanged} />
                      ))}
                    </ul>
                  </div>
                );
              })
            )}
          </Card>
        );
      })}
    </>
  );
}
