import { lazy, Suspense, useEffect, useState } from "react";
import { api, type Category, type Item, type Profile } from "./api";
import { Library } from "./components/Library";
import { NowPlaying } from "./components/NowPlaying";
import { ProfilePicker } from "./components/ProfilePicker";
import { useStatus, useToast } from "./hooks";
import { strings } from "./strings";

const PROFILE_KEY = "sonoskids-profile";

// The hidden admin page is its own chunk, so the kids' bundle stays small.
const AdminApp = lazy(() => import("./admin/AdminApp"));

/** `/admin` is the grown-ups' page; everything else is the kids' player. */
export default function App() {
  if (window.location.pathname.startsWith("/admin")) {
    return (
      <Suspense fallback={null}>
        <AdminApp />
      </Suspense>
    );
  }
  return <KidsApp />;
}

function KidsApp() {
  const [profiles, setProfiles] = useState<Profile[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [profileId, setProfileId] = useState<number | null>(
    () => Number(localStorage.getItem(PROFILE_KEY)) || null,
  );

  const [attempt, setAttempt] = useState(0);

  useEffect(() => {
    let alive = true;
    api
      .profiles()
      .then((p) => alive && setProfiles(p))
      .catch(() => alive && setFailed(true));
    return () => {
      alive = false;
    };
  }, [attempt]);

  if (failed) {
    return (
      <main className="flex h-full flex-col items-center justify-center gap-6 bg-slate-950 p-6">
        <p className="text-center text-3xl font-extrabold text-slate-100">
          {strings.cantReachServer}
        </p>
        <button
          onClick={() => {
            setFailed(false);
            setAttempt((a) => a + 1);
          }}
          className="rounded-full bg-sky-600 px-8 py-4 text-2xl font-extrabold text-white shadow-lg active:scale-95"
        >
          {strings.tryAgain}
        </button>
      </main>
    );
  }
  if (!profiles) {
    return (
      <main className="flex h-full items-center justify-center bg-slate-950 text-2xl font-bold text-slate-400">
        {strings.loading}
      </main>
    );
  }

  const profile = profiles.find((p) => p.id === profileId);
  if (!profile) {
    return (
      <ProfilePicker
        profiles={profiles}
        onPick={(p) => {
          localStorage.setItem(PROFILE_KEY, String(p.id));
          setProfileId(p.id);
        }}
      />
    );
  }
  return (
    <Session
      key={profile.id}
      profile={profile}
      onSwitch={() => {
        localStorage.removeItem(PROFILE_KEY);
        setProfileId(null);
      }}
    />
  );
}

/** Everything a signed-in kid does: browse, start something, control playback. */
function Session({ profile, onSwitch }: { profile: Profile; onSwitch: () => void }) {
  const [screen, setScreen] = useState<"library" | "player">("library");
  const [current, setCurrent] = useState<Item | null>(null);
  // Where the kid is in the library. It lives here, not in the Library screen,
  // because that screen is replaced by the player: coming back should land in
  // the same series, ready for the next episode.
  const [view, setView] = useState<{ tab: Category; artistKey: string | null }>({
    tab: "music",
    artistKey: null,
  });
  const [startingId, setStartingId] = useState<number | null>(null);
  const { status, setStatus, refresh } = useStatus(profile.id, screen === "player" ? 2000 : 5000);
  const toast = useToast();

  /** Runs a speaker command, then refreshes the status shortly after. */
  async function act(fn: () => Promise<unknown>, refreshAfterMs = 600) {
    try {
      await fn();
      setTimeout(refresh, refreshAfterMs);
    } catch {
      toast.show(strings.speakerError);
      refresh();
    }
  }

  async function play(item: Item) {
    if (startingId !== null) return;
    setStartingId(item.id);
    try {
      await api.play(profile.id, item.id);
      setCurrent(item);
      setScreen("player");
      setTimeout(refresh, 1500);
    } catch {
      toast.show(strings.playError);
    } finally {
      setStartingId(null);
    }
  }

  /** Start what is playing over from the beginning, forgetting where the kid was. */
  async function restart() {
    if (!current) return;
    try {
      await api.play(profile.id, current.id, true);
      setTimeout(refresh, 1500);
    } catch {
      toast.show(strings.playError);
    }
  }

  function togglePlay() {
    const playing = status?.state === "PLAYING" || status?.state === "TRANSITIONING";
    setStatus((s) => (s ? { ...s, state: playing ? "PAUSED_PLAYBACK" : "PLAYING" } : s)); // instant feedback
    act(() => (playing ? api.pause(profile.id) : api.resume(profile.id)), 400);
  }

  function changeVolume(delta: number) {
    if (!status) return;
    const level = Math.max(0, Math.min(status.max_volume, status.volume + delta));
    setStatus({ ...status, volume: level });
    act(() => api.volume(profile.id, level), 400);
  }

  return (
    <>
      {screen === "player" ? (
        <NowPlaying
          profile={profile}
          status={status}
          item={current}
          onBack={() => setScreen("library")}
          onPrevious={() => act(() => api.previous(profile.id))}
          onNext={() => act(() => api.next(profile.id))}
          onTogglePlay={togglePlay}
          onVolume={changeVolume}
          onRestart={current ? restart : undefined}
        />
      ) : (
        <Library
          profile={profile}
          startingId={startingId}
          status={status}
          onPlay={play}
          onOpenPlayer={() => setScreen("player")}
          onTogglePlay={togglePlay}
          onSwitch={onSwitch}
          tab={view.tab}
          artistKey={view.artistKey}
          onNavigate={(tab, artistKey) => setView({ tab, artistKey })}
        />
      )}
      {toast.message && (
        <div
          role="alert"
          className="fixed top-3 left-1/2 z-50 max-w-[90vw] -translate-x-1/2 animate-slide-down rounded-full bg-rose-600 px-6 py-3 text-center text-lg font-extrabold text-white shadow-2xl"
        >
          {toast.message}
        </div>
      )}
    </>
  );
}
