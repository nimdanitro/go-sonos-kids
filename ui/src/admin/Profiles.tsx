import { useState, type FormEvent } from "react";
import { Initial } from "../components/Cover";
import { accentFor } from "../hooks";
import { admin } from "../strings";
import type { AdminApi, AdminProfile, Room } from "./adminApi";
import { Button, Card, Field, Select, TextInput } from "./ui";

interface Props {
  api: AdminApi;
  profiles: AdminProfile[];
  rooms: Room[];
  notify: (text: string, ok?: boolean) => void;
  run: <T>(fn: () => Promise<T>) => Promise<T | undefined>;
  onChanged: () => void;
}

/** Room names for a dropdown; keeps a profile's current speaker even if it is offline. */
function SpeakerSelect({
  rooms,
  value,
  onChange,
}: {
  rooms: Room[];
  value: string;
  onChange: (v: string) => void;
}) {
  const known = rooms.some((r) => r.name === value);
  return (
    <Select required value={value} onChange={(e) => onChange(e.target.value)}>
      {value === "" && <option value="">{rooms.length ? "–" : admin.noSpeakers}</option>}
      {!known && value !== "" && <option value={value}>{value}</option>}
      {rooms.map((r) => (
        <option key={r.name} value={r.name}>
          {r.name} ({r.model})
        </option>
      ))}
    </Select>
  );
}

function ProfileRow({
  api,
  profile,
  rooms,
  notify,
  run,
  onChanged,
}: Props & { profile: AdminProfile }) {
  const [name, setName] = useState(profile.name);
  const [speaker, setSpeaker] = useState(profile.speaker);
  const [volume, setVolume] = useState(profile.max_volume);
  const dirty =
    name !== profile.name || speaker !== profile.speaker || volume !== profile.max_volume;

  async function save(e: FormEvent) {
    e.preventDefault();
    const ok = await run(() =>
      api.updateProfile(profile.id, { name, speaker, max_volume: volume }),
    );
    if (ok) {
      notify(admin.saved);
      onChanged();
    }
  }

  async function remove() {
    if (!window.confirm(admin.confirmDeleteProfile(profile.name))) return;
    await run(() => api.deleteProfile(profile.id));
    onChanged();
  }

  return (
    <form
      onSubmit={save}
      style={{ ["--accent" as string]: accentFor(profile.id) }}
      className="grid items-end gap-3 rounded-2xl bg-slate-900/60 p-3 sm:grid-cols-[auto_1fr_1.4fr_9.5rem_auto]"
    >
      <span className="flex size-12 items-center justify-center rounded-full border-4 border-(--accent) bg-slate-800 text-2xl text-(--accent)">
        <Initial name={profile.name} />
      </span>
      <Field label={admin.name}>
        <TextInput required value={name} onChange={(e) => setName(e.target.value)} />
      </Field>
      <Field label={admin.speaker}>
        <SpeakerSelect rooms={rooms} value={speaker} onChange={setSpeaker} />
      </Field>
      <Field label={admin.maxVolume}>
        <TextInput
          type="number"
          min={0}
          max={100}
          value={volume}
          onChange={(e) => setVolume(Number(e.target.value))}
        />
      </Field>
      <div className="flex gap-2">
        <Button type="submit" tone="primary" disabled={!dirty}>
          {admin.save}
        </Button>
        <Button type="button" tone="danger" onClick={remove}>
          {admin.delete}
        </Button>
      </div>
    </form>
  );
}

export function Profiles(props: Props) {
  const { api, profiles, rooms, run, notify, onChanged } = props;
  const [name, setName] = useState("");
  const [speaker, setSpeaker] = useState("");
  const [volume, setVolume] = useState(40);

  async function create(e: FormEvent) {
    e.preventDefault();
    const chosen = speaker || rooms[0]?.name || "";
    const p = await run(() => api.createProfile({ name, speaker: chosen, max_volume: volume }));
    if (p) {
      setName("");
      notify(admin.saved);
      onChanged();
    }
  }

  return (
    <Card title={admin.profilesTitle}>
      <div className="space-y-3">
        {profiles.map((p) => (
          <ProfileRow
            key={`${p.id}-${p.name}-${p.speaker}-${p.max_volume}`}
            {...props}
            profile={p}
          />
        ))}
      </div>

      <form
        onSubmit={create}
        className="mt-5 grid items-end gap-3 border-t border-slate-700 pt-4 sm:grid-cols-[1fr_1.4fr_9.5rem_auto]"
      >
        <Field label={admin.name}>
          <TextInput required value={name} onChange={(e) => setName(e.target.value)} />
        </Field>
        <Field label={admin.speaker}>
          <SpeakerSelect
            rooms={rooms}
            value={speaker || rooms[0]?.name || ""}
            onChange={setSpeaker}
          />
        </Field>
        <Field label={admin.maxVolume}>
          <TextInput
            type="number"
            min={0}
            max={100}
            value={volume}
            onChange={(e) => setVolume(Number(e.target.value))}
          />
        </Field>
        <Button type="submit" tone="primary">
          {admin.addProfile}
        </Button>
      </form>
    </Card>
  );
}
