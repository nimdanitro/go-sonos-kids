import { useEffect, useState } from "react";
import type { Item, Profile, Status } from "../api";
import { accentFor } from "../hooks";
import { strings } from "../strings";
import { Cover } from "./Cover";
import { ProgressBar } from "./ProgressBar";
import {
  BackIcon,
  CheckIcon,
  CloseIcon,
  MinusIcon,
  NextIcon,
  PauseIcon,
  PlayIcon,
  PlusIcon,
  PrevIcon,
  RestartIcon,
  SpeakerEmptyIcon,
  SpeakerIcon,
} from "./icons";

const SEGMENTS = 10;

interface Props {
  profile: Profile;
  status: Status | null;
  /** What the kid tapped to start playback; used until the speaker reports a track. */
  item: Item | null;
  onBack: () => void;
  onPrevious: () => void;
  onNext: () => void;
  onTogglePlay: () => void;
  onVolume: (delta: number) => void;
  /** Start what is playing over from the beginning. Omitted when it is not known what is playing. */
  onRestart?: () => void;
}

const VOLUME_STEP = 5;

export function NowPlaying({
  profile,
  status,
  item,
  onBack,
  onPrevious,
  onNext,
  onTogglePlay,
  onVolume,
  onRestart,
}: Props) {
  const [confirming, setConfirming] = useState(false);
  useEffect(() => {
    if (!confirming) return;
    const close = (e: KeyboardEvent) => e.key === "Escape" && setConfirming(false);
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, [confirming]);

  const playing = status?.state === "PLAYING" || status?.state === "TRANSITIONING";
  const title = status?.title || item?.title || strings.nothingPlaying;
  const sub = [status?.artist, status?.album || item?.title].filter(Boolean).join(" · ");
  const cover = status?.cover_url || item?.cover_url;
  const max = status?.max_volume || profile.max_volume || 1;
  const volume = status?.volume ?? 0;
  const filled = Math.round((Math.min(volume, max) / max) * SEGMENTS);

  const round =
    "flex shrink-0 items-center justify-center rounded-full shadow-lg transition active:scale-90 disabled:opacity-30";

  return (
    <main
      style={{ ["--accent" as string]: accentFor(profile.id) }}
      className="relative flex h-full flex-col items-center gap-3 bg-linear-to-br from-slate-950 via-slate-900 to-indigo-950 p-4 landscape:flex-row landscape:gap-[4vw] landscape:px-[4vw]"
    >
      <button
        onClick={onBack}
        aria-label={strings.back}
        className="absolute top-3 left-3 z-10 flex h-14 items-center gap-1 rounded-full bg-slate-800/90 pr-5 pl-3 text-xl font-extrabold text-slate-100 shadow-lg transition active:scale-95"
      >
        <BackIcon className="size-7" />
        {strings.back}
      </button>

      {onRestart && (
        <button
          onClick={() => setConfirming(true)}
          className="absolute top-3 right-3 z-10 flex h-14 items-center gap-2 rounded-full bg-slate-800/90 pr-5 pl-4 text-xl font-extrabold text-slate-100 shadow-lg transition active:scale-95"
        >
          <RestartIcon className="size-7" />
          {strings.restart}
        </button>
      )}

      {confirming && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4">
          {/* Tapping outside the question means "no". Hidden from screen readers, who have the real "no" button. */}
          <button
            type="button"
            aria-hidden
            tabIndex={-1}
            onClick={() => setConfirming(false)}
            className="absolute inset-0 cursor-default bg-black/65"
          />
          <dialog
            open
            aria-labelledby="restart-title"
            className="relative m-0 flex w-full max-w-md animate-pop flex-col items-center gap-4 rounded-[2rem] border-0 bg-slate-800 p-6 text-slate-50 shadow-2xl ring-4 ring-slate-700"
          >
            <RestartIcon className="size-14 text-(--accent)" />
            <h2 id="restart-title" className="text-center text-3xl font-extrabold">
              {strings.restartTitle}
            </h2>
            <div className="flex w-full gap-4">
              <button
                onClick={() => setConfirming(false)}
                className="flex h-24 flex-1 flex-col items-center justify-center gap-1 rounded-3xl bg-slate-700 text-xl font-extrabold text-slate-100 shadow-lg transition active:scale-95"
              >
                <CloseIcon className="size-9" />
                {strings.restartNo}
              </button>
              <button
                onClick={() => {
                  setConfirming(false);
                  onRestart?.();
                }}
                className="flex h-24 flex-1 flex-col items-center justify-center gap-1 rounded-3xl bg-emerald-500 text-xl font-extrabold text-white shadow-lg transition active:scale-95"
              >
                <CheckIcon className="size-9" />
                {strings.restartYes}
              </button>
            </div>
          </dialog>
        </div>
      )}

      <div className="flex shrink-0 items-center justify-center portrait:mt-14 landscape:h-full">
        <div className="relative animate-pop overflow-hidden rounded-[2rem] bg-slate-800 shadow-2xl ring-8 shadow-black/60 ring-slate-700 portrait:size-[min(40vh,82vw)] landscape:size-[min(66vh,40vw)]">
          <Cover url={cover} title={title} className="size-full text-8xl" />
          {status?.fraction !== undefined && <ProgressBar fraction={status.fraction} />}
        </div>
      </div>

      <div className="flex min-h-0 w-full min-w-0 flex-1 flex-col justify-center gap-[2.5vh] landscape:pt-12">
        <div className="min-w-0 text-center">
          <h1 className="line-clamp-2 text-[clamp(1.5rem,5vmin,2.75rem)] leading-tight font-extrabold text-slate-50">
            {title}
          </h1>
          <p className="mt-1 line-clamp-2 text-[clamp(1rem,2.8vmin,1.5rem)] font-bold text-slate-400">
            {sub}
          </p>
        </div>

        {/* Both control rows share one width and are centred under the title, so their edges line up. */}
        <div className="mx-auto flex w-full max-w-lg items-center justify-between">
          <button
            onClick={onPrevious}
            aria-label={strings.previous}
            className={`${round} size-[clamp(4.5rem,13vmin,6.5rem)] bg-slate-800 text-slate-100`}
          >
            <PrevIcon className="size-[55%]" />
          </button>
          <button
            onClick={onTogglePlay}
            aria-label={playing ? strings.pause : strings.play}
            className={`${round} size-[clamp(5.5rem,17vmin,8.5rem)] bg-(--accent) text-white`}
          >
            {playing ? <PauseIcon className="size-[55%]" /> : <PlayIcon className="size-[55%]" />}
          </button>
          <button
            onClick={onNext}
            aria-label={strings.next}
            className={`${round} size-[clamp(4.5rem,13vmin,6.5rem)] bg-slate-800 text-slate-100`}
          >
            <NextIcon className="size-[55%]" />
          </button>
        </div>

        <div className="mx-auto flex w-full max-w-lg items-center justify-between gap-3">
          <button
            onClick={() => onVolume(-VOLUME_STEP)}
            disabled={volume <= 0}
            aria-label={strings.quieter}
            className={`${round} size-[clamp(3.5rem,10vmin,5rem)] bg-slate-800 text-slate-100`}
          >
            <MinusIcon className="size-[60%]" />
          </button>
          <div className="flex min-w-0 flex-1 items-center gap-2 rounded-full bg-slate-800/80 px-6 py-3 shadow-inner">
            <meter
              className="sr-only"
              min={0}
              max={max}
              value={volume}
              aria-label={strings.volume}
            />
            {/* quiet end: a speaker with no sound waves */}
            <SpeakerEmptyIcon className="size-6 shrink-0 text-slate-300" />
            {/* the bars take the space between the icons and sit centred in it */}
            <div className="flex flex-1 items-center justify-center gap-1" aria-hidden>
              {Array.from({ length: SEGMENTS }, (_, i) => (
                <span
                  key={i}
                  className={`w-[clamp(0.5rem,1.6vmin,1rem)] rounded-full transition-colors ${i < filled ? "bg-(--accent)" : "bg-slate-600"}`}
                  style={{ height: `${1.1 + i * 0.28}rem` }}
                />
              ))}
            </div>
            {/* loud end: a speaker with full sound waves */}
            <SpeakerIcon className="size-6 shrink-0 text-slate-300" />
          </div>
          <button
            onClick={() => onVolume(VOLUME_STEP)}
            disabled={volume >= max}
            aria-label={strings.louder}
            className={`${round} size-[clamp(3.5rem,10vmin,5rem)] bg-slate-800 text-slate-100`}
          >
            <PlusIcon className="size-[60%]" />
          </button>
        </div>
      </div>
    </main>
  );
}
