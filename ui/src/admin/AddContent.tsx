import { useState, type FormEvent } from "react";
import type { Category } from "../api";
import { Cover, Initial } from "../components/Cover";
import { accentFor } from "../hooks";
import { admin } from "../strings";
import type { AdminApi, AdminProfile, BatchItem, Discography, Preview } from "./adminApi";
import { DiscographyPicker } from "./Discography";
import { Button, Card, Field, TextInput } from "./ui";

interface Props {
  api: AdminApi;
  profiles: AdminProfile[];
  notify: (text: string, ok?: boolean) => void;
  run: <T>(fn: () => Promise<T>) => Promise<T | undefined>;
  onAdded: () => void;
}

const CATEGORIES = [
  ["music", admin.categoryMusic],
  ["audiobook", admin.categoryAudiobook],
  ["playlist", admin.categoryPlaylist],
] as const;

/** An artist page (or spotify:artist: URI) means "import the whole discography". */
const looksLikeArtist = (link: string) => /\/artist\/|^spotify:artist:/i.test(link.trim());

export function AddContent({ api, profiles, notify, run, onAdded }: Props) {
  const [url, setUrl] = useState("");
  const [category, setCategory] = useState<Category>("music");
  const [title, setTitle] = useState("");
  const [cover, setCover] = useState("");
  const [artist, setArtist] = useState("");
  const [excluded, setExcluded] = useState<number[]>([]);
  const [preview, setPreview] = useState<Preview | null>(null);
  const [discography, setDiscography] = useState<Discography | null>(null);
  const [busy, setBusy] = useState(false);

  const isArtist = looksLikeArtist(url);
  const selected = profiles.filter((p) => !excluded.includes(p.id));
  const toggle = (id: number) =>
    setExcluded((ex) => (ex.includes(id) ? ex.filter((x) => x !== id) : [...ex, id]));

  function changeUrl(value: string) {
    setUrl(value);
    setPreview(null);
    setDiscography(null);
    // The title, artist and cover fields are overrides for one specific link,
    // so they must not carry over to the next one.
    setTitle("");
    setArtist("");
    setCover("");
  }

  // The preview only shows what the server found; it does not fill the override
  // fields, so anything left empty stays "automatic".
  async function showPreview() {
    const p = await run(() => api.preview(url));
    if (!p) return;
    setPreview(p);
    setCategory(p.category); // playlists are filed under Playlists by default
    if (!p.title) notify(admin.previewNoTitle, false);
  }

  async function loadDiscography() {
    setBusy(true);
    const d = await run(() => api.discography(url));
    setBusy(false);
    if (d) setDiscography(d);
  }

  async function addSingle() {
    setBusy(true);
    const res = await run(() =>
      api.addItems({
        url,
        category,
        profile_ids: selected.map((p) => p.id),
        title: title || undefined,
        cover_url: cover || undefined,
        artist: artist || undefined,
      }),
    );
    setBusy(false);
    if (!res) return;
    const nameOf = (id: number) => profiles.find((p) => p.id === id)?.name ?? `#${id}`;
    const problems = res.errors.map((er) => `${nameOf(er.profile_id)}: ${er.error}`).join(" · ");
    if (res.items.length > 0) {
      notify(admin.added(res.items.length) + (problems ? ` (${problems})` : ""));
      reset();
    } else {
      notify(problems || admin.chooseProfile, false);
    }
  }

  async function addAlbums(items: BatchItem[]) {
    setBusy(true);
    const res = await run(() =>
      api.addBatch({ category, profile_ids: selected.map((p) => p.id), items }),
    );
    setBusy(false);
    if (!res) return;
    if (res.created > 0) {
      notify(admin.importSummary(items.length, selected.length, res.created, res.duplicates));
      reset();
    } else {
      notify(res.errors[0]?.error ?? admin.allPresent, false);
    }
  }

  function reset() {
    setUrl("");
    setTitle("");
    setCover("");
    setArtist("");
    setPreview(null);
    setDiscography(null);
    onAdded();
  }

  function submit(e: FormEvent) {
    e.preventDefault();
    if (selected.length === 0) return notify(admin.chooseProfile, false);
    if (isArtist) void loadDiscography();
    else void addSingle();
  }

  return (
    <Card title={admin.addTitle}>
      <form onSubmit={submit} className="space-y-4">
        <Field label={admin.linkLabel}>
          <TextInput
            type="url"
            required
            value={url}
            onChange={(e) => changeUrl(e.target.value)}
            placeholder={admin.linkPlaceholder}
          />
        </Field>

        <div className={`grid gap-4 ${isArtist ? "" : "sm:grid-cols-[auto_1fr_1fr_1fr]"}`}>
          <div>
            <span className="mb-1 block text-sm font-bold text-slate-300">{admin.category}</span>
            <div className="inline-flex gap-1 rounded-xl bg-slate-900 p-1">
              {CATEGORIES.map(([key, label]) => (
                <button
                  key={key}
                  type="button"
                  aria-pressed={category === key}
                  onClick={() => setCategory(key)}
                  className={`rounded-lg px-4 py-1.5 font-bold transition ${
                    category === key ? "bg-sky-500 text-white" : "text-slate-300"
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
          {!isArtist && (
            <>
              <Field label={admin.titleOptional}>
                <TextInput
                  value={title}
                  placeholder={preview?.title}
                  onChange={(e) => setTitle(e.target.value)}
                />
              </Field>
              <Field label={admin.artistOptional}>
                <TextInput
                  value={artist}
                  placeholder={preview?.artist}
                  onChange={(e) => setArtist(e.target.value)}
                />
              </Field>
              <Field label={admin.coverOptional}>
                <TextInput
                  type="url"
                  value={cover}
                  placeholder={preview?.cover_url}
                  onChange={(e) => setCover(e.target.value)}
                />
              </Field>
            </>
          )}
        </div>

        <div>
          <span className="mb-2 block text-sm font-bold text-slate-300">{admin.forWhom}</span>
          <div className="flex flex-wrap gap-2">
            {profiles.map((p) => {
              const on = !excluded.includes(p.id);
              return (
                <button
                  key={p.id}
                  type="button"
                  aria-pressed={on}
                  onClick={() => toggle(p.id)}
                  style={{ ["--accent" as string]: accentFor(p.id) }}
                  className={`flex items-center gap-2 rounded-full border-4 py-1 pr-4 pl-1 font-bold transition active:scale-95 ${
                    on
                      ? "border-(--accent) bg-slate-800 text-slate-50"
                      : "border-slate-700 bg-slate-900 text-slate-500"
                  }`}
                >
                  <span
                    className={`flex size-9 items-center justify-center rounded-full text-xl ${
                      on ? "bg-(--accent) text-white" : "bg-slate-800"
                    }`}
                  >
                    <Initial name={p.name} />
                  </span>
                  {p.name}
                </button>
              );
            })}
          </div>
        </div>

        {!isArtist && preview && (
          <div className="flex items-center gap-3 rounded-2xl bg-slate-900 p-3">
            <Cover
              url={preview.cover_url}
              title={preview.title}
              className="size-20 shrink-0 rounded-xl text-3xl"
            />
            <div className="min-w-0">
              <p className="truncate text-lg font-extrabold text-slate-50">
                {preview.title || "–"}
              </p>
              <p className="text-sm font-bold text-slate-400">
                {[
                  preview.artist,
                  preview.release_date.slice(0, 4),
                  `${preview.service} ${preview.kind}`,
                ]
                  .filter(Boolean)
                  .join(" · ")}
              </p>
            </div>
          </div>
        )}

        {isArtist ? (
          <Button type="submit" tone="primary" disabled={busy || !url}>
            {busy ? admin.loadingDiscography : admin.loadDiscography}
          </Button>
        ) : (
          <div className="flex gap-2">
            <Button type="button" onClick={showPreview} disabled={!url}>
              {admin.preview}
            </Button>
            <Button type="submit" tone="primary" disabled={busy || !url}>
              {busy ? admin.adding : admin.add}
            </Button>
          </div>
        )}
      </form>

      {isArtist && discography && (
        <div className="mt-4">
          <DiscographyPicker key={url} data={discography} busy={busy} onAdd={addAlbums} />
        </div>
      )}
    </Card>
  );
}
