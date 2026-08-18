import type { ReactNode } from "react";

export type IconName =
  | "agents"
  | "archive"
  | "environments"
  | "external"
  | "panelClose"
  | "panelOpen"
  | "plus"
  | "workspaces";

interface IconProps {
  name: IconName;
}

export function Icon({ name }: IconProps) {
  const paths: Record<IconName, ReactNode> = {
    agents: (
      <>
        <path d="M12 3.5 13.4 8l4.6 1.4-4.6 1.4L12 15.5l-1.4-4.7L6 9.4 10.6 8 12 3.5Z" />
        <path d="m18 15 .7 2.3L21 18l-2.3.7L18 21l-.7-2.3L15 18l2.3-.7L18 15Z" />
      </>
    ),
    archive: (
      <>
        <path d="M4 7.5h16" />
        <path d="M5.5 7.5v11h13v-11" />
        <path d="M9.5 11.5h5" />
        <path d="M4.5 3.5h15v4h-15z" />
      </>
    ),
    environments: (
      <>
        <rect x="4" y="4" width="16" height="6" rx="1.5" />
        <rect x="4" y="14" width="16" height="6" rx="1.5" />
        <path d="M8 7h.01M8 17h.01M12 7h5M12 17h5" />
      </>
    ),
    external: (
      <>
        <path d="M14 5h5v5" />
        <path d="m19 5-8 8" />
        <path d="M19 14v4a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V6a1 1 0 0 1 1-1h4" />
      </>
    ),
    panelClose: (
      <>
        <rect x="4" y="5" width="16" height="14" rx="2" />
        <path d="M9 5v14M14 9l-3 3 3 3" />
      </>
    ),
    panelOpen: (
      <>
        <rect x="4" y="5" width="16" height="14" rx="2" />
        <path d="M9 5v14m3-10 3 3-3 3" />
      </>
    ),
    plus: <path d="M12 5v14M5 12h14" />,
    workspaces: (
      <>
        <path d="M3.5 6.5h6l2 2h9v10h-17z" />
        <path d="M3.5 9h17" />
      </>
    ),
  };

  return (
    <svg
      aria-hidden="true"
      className="ui-icon"
      fill="none"
      viewBox="0 0 24 24"
      stroke="currentColor"
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeWidth="1.7"
    >
      {paths[name]}
    </svg>
  );
}
