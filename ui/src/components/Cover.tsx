import { useState } from "react";

/** Cover art with a colourful fallback showing the title's first letter. */
export function Cover({
  url,
  title,
  className = "",
}: {
  url?: string;
  title: string;
  className?: string;
}) {
  const [failed, setFailed] = useState(false);
  if (!url || failed) {
    return (
      <div
        className={`flex items-center justify-center bg-linear-to-br from-amber-400 to-pink-500 text-6xl font-extrabold text-white/90 ${className}`}
        aria-hidden
      >
        {firstLetter(title) || "♪"}
      </div>
    );
  }
  return (
    <img
      src={url}
      alt=""
      className={`object-cover ${className}`}
      onError={() => setFailed(true)}
      draggable={false}
    />
  );
}

/** First visible character, uppercased (handles multi-byte letters). */
export function firstLetter(name: string) {
  return Array.from(name.trim())[0]?.toUpperCase() ?? "";
}

/** A profile's avatar: just the first letter of their name. */
export function Initial({ name, className = "" }: { name: string; className?: string }) {
  return (
    <span className={`leading-none font-black ${className}`} aria-hidden>
      {firstLetter(name) || "?"}
    </span>
  );
}
