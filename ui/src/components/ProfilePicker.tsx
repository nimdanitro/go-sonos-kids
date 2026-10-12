import type { Profile } from "../api";
import { accentFor } from "../hooks";
import { strings } from "../strings";
import { Initial } from "./Cover";

export function ProfilePicker({
  profiles,
  onPick,
}: {
  profiles: Profile[];
  onPick: (p: Profile) => void;
}) {
  return (
    <main className="flex h-full flex-col items-center justify-center gap-[4vmin] bg-linear-to-br from-slate-950 via-slate-900 to-indigo-950 p-6">
      <h1 className="text-[clamp(1.75rem,6vmin,3.5rem)] font-extrabold text-slate-50">
        {strings.whoIsListening}
      </h1>
      <div className="flex flex-wrap items-center justify-center gap-[3vmin]">
        {profiles.map((p) => (
          <button
            key={p.id}
            onClick={() => onPick(p)}
            aria-label={p.name}
            style={{ ["--accent" as string]: accentFor(p.id) }}
            className="flex size-[clamp(9rem,34vmin,18rem)] animate-pop items-center justify-center rounded-[2.5rem] border-[6px] border-(--accent) bg-slate-800 shadow-xl shadow-black/50 transition active:scale-95"
          >
            <Initial name={p.name} className="text-[clamp(5rem,22vmin,11rem)] text-(--accent)" />
          </button>
        ))}
      </div>
    </main>
  );
}
