import type { ComponentProps, ReactNode } from "react";

// Shared look for the admin page: the same dark slate palette as the kids' UI.

export const inputClass =
  "w-full rounded-xl border border-slate-600 bg-slate-900 px-3 py-2 text-slate-50 placeholder:text-slate-500 focus:border-sky-500 focus:ring-2 focus:ring-sky-500/40 focus:outline-none";

export function Card({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="rounded-3xl bg-slate-800/80 p-5 shadow-xl ring-1 shadow-black/40 ring-slate-700">
      <h2 className="mb-4 text-xl font-extrabold text-slate-50">{title}</h2>
      {children}
    </section>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block text-sm font-bold text-slate-300">
      <span className="mb-1 block">{label}</span>
      {children}
    </label>
  );
}

export function TextInput(props: ComponentProps<"input">) {
  return <input {...props} className={`${inputClass} ${props.className ?? ""}`} />;
}

export function Select(props: ComponentProps<"select">) {
  return <select {...props} className={`${inputClass} ${props.className ?? ""}`} />;
}

type Tone = "primary" | "neutral" | "danger";
const tones: Record<Tone, string> = {
  primary: "bg-sky-500 text-white hover:bg-sky-400",
  neutral: "bg-slate-700 text-slate-100 hover:bg-slate-600",
  danger: "bg-rose-600/90 text-white hover:bg-rose-500",
};

export function Button({
  tone = "neutral",
  className = "",
  ...props
}: { tone?: Tone } & ComponentProps<"button">) {
  return (
    <button
      {...props}
      className={`rounded-xl px-4 py-2 font-bold transition active:scale-95 disabled:opacity-40 ${tones[tone]} ${className}`}
    />
  );
}
