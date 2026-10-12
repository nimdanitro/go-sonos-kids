import { useCallback, useEffect, useState } from "react";
import { api, type Status } from "./api";

/** Polls the speaker status for a profile. */
export function useStatus(profileId: number, intervalMs: number) {
  const [status, setStatus] = useState<Status | null>(null);

  // For on-demand refreshes after an action.
  const refresh = useCallback(async () => {
    try {
      setStatus(await api.status(profileId));
    } catch {
      // Keep showing the last known status; actions report their own errors.
    }
  }, [profileId]);

  useEffect(() => {
    let alive = true;
    const tick = () =>
      api
        .status(profileId)
        .then((s) => alive && setStatus(s))
        .catch(() => {});
    tick();
    const t = setInterval(tick, intervalMs);
    return () => {
      alive = false;
      clearInterval(t);
    };
  }, [profileId, intervalMs]);

  return { status, setStatus, refresh };
}

/** A message that disappears by itself. */
export function useToast(ms = 3500) {
  const [message, setMessage] = useState<string | null>(null);
  useEffect(() => {
    if (!message) return;
    const t = setTimeout(() => setMessage(null), ms);
    return () => clearTimeout(t);
  }, [message, ms]);
  return { message, show: setMessage };
}

// Friendly accent colours, picked per profile so each kid has "their" colour.
const ACCENTS = ["#f97316", "#0ea5e9", "#a855f7", "#10b981", "#ec4899", "#eab308"];
export const accentFor = (profileId: number) => ACCENTS[profileId % ACCENTS.length];
