/**
 * A thick bar along the bottom edge of a cover showing how far along something
 * is. Put it inside a cover that is `relative overflow-hidden`; it takes the
 * accent colour from `--accent`.
 */
export function ProgressBar({ fraction }: { fraction: number }) {
  const percent = Math.round(Math.min(1, Math.max(0, fraction)) * 100);
  return (
    <>
      <div
        className="absolute inset-x-0 bottom-0 h-3.5 border-t border-white/25 bg-slate-950/75"
        aria-hidden
      >
        <div
          className="h-full bg-(--accent) transition-[width] duration-700"
          style={{ width: `${percent}%` }}
        />
      </div>
      <progress className="sr-only" max={100} value={percent} />
    </>
  );
}
