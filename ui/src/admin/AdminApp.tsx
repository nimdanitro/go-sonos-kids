import { useCallback, useEffect, useMemo, useState, type FormEvent } from "react";
import type { Item } from "../api";
import { admin } from "../strings";
import { AddContent } from "./AddContent";
import { adminApi, ApiError, AuthError, type AdminProfile, type Room } from "./adminApi";
import { Libraries } from "./Libraries";
import { Profiles } from "./Profiles";
import { Button, Card, Field, TextInput } from "./ui";

const TOKEN_KEY = "sonoskids-admin-token";

const page =
  "h-full select-text overflow-y-auto bg-linear-to-br from-slate-950 via-slate-900 to-indigo-950 text-slate-50";

export default function AdminApp() {
  const [token, setToken] = useState(() => localStorage.getItem(TOKEN_KEY) ?? "");
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    document.title = "Admin";
  }, []);

  const logout = useCallback((message: string | null = null) => {
    localStorage.removeItem(TOKEN_KEY);
    setToken("");
    setError(message);
  }, []);

  if (!token) {
    return (
      <TokenGate
        error={error}
        onSubmit={(t) => {
          localStorage.setItem(TOKEN_KEY, t);
          setError(null);
          setToken(t);
        }}
      />
    );
  }
  return (
    <Dashboard
      token={token}
      onLogout={() => logout()}
      onAuthError={(msg) =>
        logout(/disabled/i.test(msg) ? admin.adminDisabled : admin.invalidToken)
      }
    />
  );
}

function TokenGate({ error, onSubmit }: { error: string | null; onSubmit: (t: string) => void }) {
  const [value, setValue] = useState("");
  function submit(e: FormEvent) {
    e.preventDefault();
    if (value) onSubmit(value);
  }
  return (
    <main className={`${page} flex items-center justify-center p-4`}>
      <form onSubmit={submit} className="w-full max-w-sm">
        <Card title={admin.title}>
          <div className="space-y-4">
            <Field label={admin.tokenLabel}>
              <TextInput
                type="password"
                autoComplete="off"
                value={value}
                onChange={(e) => setValue(e.target.value)}
              />
            </Field>
            {error && (
              <p role="alert" className="font-bold text-rose-400">
                {error}
              </p>
            )}
            <Button type="submit" tone="primary" className="w-full">
              {admin.connect}
            </Button>
          </div>
        </Card>
      </form>
    </main>
  );
}

function Dashboard({
  token,
  onAuthError,
  onLogout,
}: {
  token: string;
  onAuthError: (message: string) => void;
  onLogout: () => void;
}) {
  const api = useMemo(() => adminApi(token), [token]);
  const [profiles, setProfiles] = useState<AdminProfile[]>([]);
  const [rooms, setRooms] = useState<Room[]>([]);
  const [libraries, setLibraries] = useState<Record<number, Item[]>>({});
  const [version, setVersion] = useState(0);
  const [banner, setBanner] = useState<{ text: string; ok: boolean } | null>(null);

  const notify = useCallback((text: string, ok = true) => setBanner({ text, ok }), []);
  const reload = useCallback(() => setVersion((v) => v + 1), []);

  useEffect(() => {
    if (!banner) return;
    const t = setTimeout(() => setBanner(null), 6000);
    return () => clearTimeout(t);
  }, [banner]);

  /** Runs an API call, turning failures into a banner (or back to the token gate). */
  const run = useCallback(
    async <T,>(fn: () => Promise<T>): Promise<T | undefined> => {
      try {
        return await fn();
      } catch (e) {
        if (e instanceof AuthError) onAuthError(e.message);
        else if (e instanceof ApiError && e.code === "not_configured")
          notify(admin.notConfigured(e.service ?? ""), false);
        else notify(e instanceof Error ? e.message : String(e), false);
        return undefined;
      }
    },
    [notify, onAuthError],
  );

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const ps = await api.profiles();
        const libs = await Promise.all(ps.map((p) => api.library(p.id)));
        if (!alive) return;
        setProfiles(ps);
        setLibraries(Object.fromEntries(ps.map((p, i) => [p.id, libs[i]])));
      } catch (e) {
        if (!alive) return;
        if (e instanceof AuthError) onAuthError(e.message);
        else notify(e instanceof Error ? e.message : String(e), false);
      }
    })();
    return () => {
      alive = false;
    };
  }, [api, version, onAuthError, notify]);

  // Finding speakers can take a few seconds on a cold start, so load it separately.
  useEffect(() => {
    let alive = true;
    api
      .speakers()
      .then((r) => alive && setRooms(r))
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, [api]);

  return (
    <main className={page}>
      <div className="mx-auto max-w-4xl space-y-5 p-4 pb-16">
        <header className="flex items-center justify-between">
          <h1 className="text-2xl font-extrabold">{admin.title}</h1>
          <Button onClick={onLogout}>{admin.disconnect}</Button>
        </header>

        <AddContent api={api} profiles={profiles} notify={notify} run={run} onAdded={reload} />
        <Profiles
          api={api}
          profiles={profiles}
          rooms={rooms}
          notify={notify}
          run={run}
          onChanged={reload}
        />
        <Libraries
          api={api}
          profiles={profiles}
          libraries={libraries}
          notify={notify}
          run={run}
          onChanged={reload}
        />
      </div>

      {banner && (
        <output
          className={`fixed top-3 left-1/2 z-50 max-w-[92vw] -translate-x-1/2 animate-slide-down rounded-full px-6 py-3 text-center font-extrabold text-white shadow-2xl ${
            banner.ok ? "bg-emerald-600" : "bg-rose-600"
          }`}
        >
          {banner.text}
        </output>
      )}
    </main>
  );
}
