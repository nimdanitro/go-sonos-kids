import type { SVGProps } from "react";

const base = (props: SVGProps<SVGSVGElement>) => ({
  viewBox: "0 0 24 24",
  fill: "currentColor",
  "aria-hidden": true,
  ...props,
});

export const PlayIcon = (p: SVGProps<SVGSVGElement>) => (
  <svg {...base(p)}>
    <path d="M8 5.5v13a1 1 0 0 0 1.5.86l10.5-6.5a1 1 0 0 0 0-1.72L9.5 4.64A1 1 0 0 0 8 5.5z" />
  </svg>
);
export const PauseIcon = (p: SVGProps<SVGSVGElement>) => (
  <svg {...base(p)}>
    <rect x="6" y="5" width="4.5" height="14" rx="1.2" />
    <rect x="13.5" y="5" width="4.5" height="14" rx="1.2" />
  </svg>
);
export const NextIcon = (p: SVGProps<SVGSVGElement>) => (
  <svg {...base(p)}>
    <path d="M5 6.4v11.2a.9.9 0 0 0 1.4.75l8.2-5.6a.9.9 0 0 0 0-1.5L6.4 5.65A.9.9 0 0 0 5 6.4z" />
    <rect x="16" y="5.5" width="3" height="13" rx="1.1" />
  </svg>
);
export const PrevIcon = (p: SVGProps<SVGSVGElement>) => (
  <svg {...base(p)}>
    <path d="M19 6.4v11.2a.9.9 0 0 1-1.4.75l-8.2-5.6a.9.9 0 0 1 0-1.5l8.2-5.6A.9.9 0 0 1 19 6.4z" />
    <rect x="5" y="5.5" width="3" height="13" rx="1.1" />
  </svg>
);
export const PlusIcon = (p: SVGProps<SVGSVGElement>) => (
  <svg {...base(p)}>
    <path d="M11 5.5a1 1 0 0 1 2 0V11h5.5a1 1 0 0 1 0 2H13v5.5a1 1 0 0 1-2 0V13H5.5a1 1 0 0 1 0-2H11z" />
  </svg>
);
export const MinusIcon = (p: SVGProps<SVGSVGElement>) => (
  <svg {...base(p)}>
    <rect x="5" y="11" width="14" height="2" rx="1" />
  </svg>
);
export const SpeakerIcon = (p: SVGProps<SVGSVGElement>) => (
  <svg {...base(p)}>
    <path d="M3 9.5v5h3.5l4.4 3.9a.7.7 0 0 0 1.1-.5V6.1a.7.7 0 0 0-1.1-.5L6.5 9.5H3z" />
    <path
      d="M15.5 8.5a5 5 0 0 1 0 7M18 6a8.5 8.5 0 0 1 0 12"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
    />
  </svg>
);
/** A speaker with no sound waves: the quiet end of the volume bar. */
export const SpeakerEmptyIcon = (p: SVGProps<SVGSVGElement>) => (
  <svg {...base(p)}>
    <path d="M3 9.5v5h3.5l4.4 3.9a.7.7 0 0 0 1.1-.5V6.1a.7.7 0 0 0-1.1-.5L6.5 9.5H3z" />
  </svg>
);
export const BackIcon = (p: SVGProps<SVGSVGElement>) => (
  <svg
    {...base(p)}
    fill="none"
    stroke="currentColor"
    strokeWidth="2.6"
    strokeLinecap="round"
    strokeLinejoin="round"
  >
    <path d="M14.5 5.5 8 12l6.5 6.5" />
  </svg>
);
export const NoteIcon = (p: SVGProps<SVGSVGElement>) => (
  <svg {...base(p)}>
    <path d="M12 3.2v10.1a4 4 0 1 0 2 3.4V7h4V3.2h-6z" />
  </svg>
);
export const BookIcon = (p: SVGProps<SVGSVGElement>) => (
  <svg {...base(p)}>
    <path d="M6 3h11a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2zm1 2v7l2.5-1.7L12 12V5H7z" />
  </svg>
);
export const ListIcon = (p: SVGProps<SVGSVGElement>) => (
  <svg {...base(p)}>
    <rect x="3" y="5" width="3" height="3" rx="1" />
    <rect x="3" y="10.5" width="3" height="3" rx="1" />
    <rect x="3" y="16" width="3" height="3" rx="1" />
    <rect x="8.5" y="5.5" width="12" height="2" rx="1" />
    <rect x="8.5" y="11" width="12" height="2" rx="1" />
    <rect x="8.5" y="16.5" width="12" height="2" rx="1" />
  </svg>
);
/** A circular arrow: start again. */
export const RestartIcon = (p: SVGProps<SVGSVGElement>) => (
  <svg
    {...base(p)}
    fill="none"
    stroke="currentColor"
    strokeWidth="2.4"
    strokeLinecap="round"
    strokeLinejoin="round"
  >
    <path d="M4 12a8 8 0 1 0 2.6-5.9" />
    <path d="M4 4.5V9h4.5" />
  </svg>
);
export const CloseIcon = (p: SVGProps<SVGSVGElement>) => (
  <svg {...base(p)} fill="none" stroke="currentColor" strokeWidth="3" strokeLinecap="round">
    <path d="M6 6l12 12M18 6 6 18" />
  </svg>
);
export const CheckIcon = (p: SVGProps<SVGSVGElement>) => (
  <svg
    {...base(p)}
    fill="none"
    stroke="currentColor"
    strokeWidth="3.2"
    strokeLinecap="round"
    strokeLinejoin="round"
  >
    <path d="M5 12.5 9.5 17 19 7.5" />
  </svg>
);
export const Spinner = (p: SVGProps<SVGSVGElement>) => (
  <svg
    {...base(p)}
    fill="none"
    stroke="currentColor"
    strokeWidth="3.5"
    strokeLinecap="round"
    className={`animate-spin ${p.className ?? ""}`}
  >
    <circle cx="12" cy="12" r="9" opacity="0.25" />
    <path d="M21 12a9 9 0 0 0-9-9" />
  </svg>
);
