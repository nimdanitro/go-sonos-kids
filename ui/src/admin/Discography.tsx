import { useState } from "react";
import { Cover } from "../components/Cover";
import { admin } from "../strings";
import type { Album, BatchItem, Discography } from "./adminApi";
import { Button } from "./ui";

const TYPE_LABELS: Record<string, string> = {
  album: admin.typeAlbum,
  single: admin.typeSingle,
  compilation: admin.typeCompilation,
};
const TYPE_ORDER = ["album", "single", "compilation"];

/** Unknown types are treated as albums. */
const typeOf = (a: Album) => (a.type in TYPE_LABELS ? a.type : "album");

/**
 * Lets the admin tick which of an artist's releases to import. Everything but
 * compilations starts selected, so usually the job is just to deselect a few.
 */
export function DiscographyPicker({
  data,
  busy,
  onAdd,
}: {
  data: Discography;
  busy: boolean;
  onAdd: (items: BatchItem[]) => void;
}) {
  const [picked, setPicked] = useState<Set<string>>(
    () => new Set(data.albums.filter((a) => typeOf(a) !== "compilation").map((a) => a.id)),
  );

  const change = (ids: string[], on: boolean) =>
    setPicked((prev) => {
      const next = new Set(prev);
      for (const id of ids) {
        if (on) next.add(id);
        else next.delete(id);
      }
      return next;
    });
  const allIds = data.albums.map((a) => a.id);
  const types = TYPE_ORDER.filter((t) => data.albums.some((a) => typeOf(a) === t));

  function add() {
    onAdd(
      data.albums
        .filter((a) => picked.has(a.id))
        .map((a) => ({
          service: data.service,
          kind: "album" as const,
          id: a.id,
          title: a.title,
          cover_url: a.cover_url,
          artist: a.artist || data.artist.name,
          release_date: a.release_date,
        })),
    );
  }

  return (
    <div className="space-y-3 rounded-2xl bg-slate-900/70 p-3">
      <div className="flex items-center gap-3">
        <Cover
          url={data.artist.image_url}
          title={data.artist.name}
          className="size-14 shrink-0 rounded-full text-2xl"
        />
        <div className="min-w-0">
          <h3 className="truncate text-lg font-extrabold text-slate-50">
            {admin.discographyOf(data.artist.name)}
          </h3>
          <p className="text-sm font-bold text-slate-400">{admin.discographyHint}</p>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <Button type="button" onClick={() => change(allIds, true)}>
          {admin.selectAll}
        </Button>
        <Button type="button" onClick={() => change(allIds, false)}>
          {admin.selectNone}
        </Button>
        <span className="mx-1 h-6 w-px bg-slate-700" aria-hidden />
        {types.map((t) => {
          const ids = data.albums.filter((a) => typeOf(a) === t).map((a) => a.id);
          const all = ids.every((id) => picked.has(id));
          return (
            <button
              key={t}
              type="button"
              aria-pressed={all}
              onClick={() => change(ids, !all)}
              className={`rounded-full border-2 px-3 py-1 text-sm font-bold transition active:scale-95 ${
                all
                  ? "border-sky-500 bg-sky-500/20 text-sky-200"
                  : "border-slate-600 text-slate-400"
              }`}
            >
              {TYPE_LABELS[t]} ({ids.length})
            </button>
          );
        })}
      </div>

      <ul className="max-h-[28rem] divide-y divide-slate-700/70 overflow-y-auto rounded-xl bg-slate-800/60">
        {data.albums.map((a) => (
          <li key={a.id}>
            <label className="flex cursor-pointer items-center gap-3 p-2 hover:bg-slate-700/40">
              <input
                type="checkbox"
                checked={picked.has(a.id)}
                onChange={(e) => change([a.id], e.target.checked)}
                className="size-6 shrink-0 accent-sky-500"
              />
              <Cover
                url={a.cover_url}
                title={a.title}
                className="size-12 shrink-0 rounded-lg text-xl"
              />
              <span className="min-w-0 flex-1">
                <span className="block truncate font-extrabold text-slate-50">{a.title}</span>
                <span className="block text-sm font-bold text-slate-400">
                  {a.release_date ? a.release_date.slice(0, 4) : "–"} · {TYPE_LABELS[typeOf(a)]}
                </span>
              </span>
            </label>
          </li>
        ))}
      </ul>

      <div className="flex items-center justify-between gap-3">
        <span className="font-bold text-slate-300">
          {admin.selectedOf(picked.size, data.albums.length)}
        </span>
        <Button type="button" tone="primary" disabled={busy || picked.size === 0} onClick={add}>
          {busy ? admin.adding : admin.addSelected(picked.size)}
        </Button>
      </div>
    </div>
  );
}
