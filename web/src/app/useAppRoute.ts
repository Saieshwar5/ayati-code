import { useCallback, useEffect, useState } from "react";

export type AppRoute =
  | { page: "workspaces" }
  | { page: "create-workspace" }
  | { page: "workspace"; workspaceID: string }
  | { page: "session"; workspaceID: string; sessionID: string }
  | { page: "archived" }
  | { page: "agents" }
  | { page: "environments" };

export function useAppRoute() {
  const [route, setRoute] = useState(() => parseRoute(window.location.pathname));

  useEffect(() => {
    const onPopState = () => setRoute(parseRoute(window.location.pathname));
    window.addEventListener("popstate", onPopState);
    return () => window.removeEventListener("popstate", onPopState);
  }, []);

  const navigate = useCallback((path: string, replace = false) => {
    if (replace) window.history.replaceState({}, "", path);
    else window.history.pushState({}, "", path);
    setRoute(parseRoute(path));
  }, []);

  return { route, navigate };
}

export function workspacePath(workspaceID: string): string {
  return `/workspaces/${encodeURIComponent(workspaceID)}`;
}

export function sessionPath(workspaceID: string, sessionID: string): string {
  return `${workspacePath(workspaceID)}/sessions/${encodeURIComponent(sessionID)}`;
}

function parseRoute(pathname: string): AppRoute {
  const parts = pathname.split("/").filter(Boolean).map(decodeURIComponent);
  if (!parts.length || (parts.length === 1 && parts[0] === "workspaces")) {
    return { page: "workspaces" };
  }
  if (parts[0] === "workspaces" && parts[1] === "new" && parts.length === 2) {
    return { page: "create-workspace" };
  }
  if (parts[0] === "workspaces" && parts[1] === "archived" && parts.length === 2) {
    return { page: "archived" };
  }
  if (parts[0] === "workspaces" && parts[1] && parts[2] === "sessions" && parts[3]) {
    return { page: "session", workspaceID: parts[1], sessionID: parts[3] };
  }
  if (parts[0] === "workspaces" && parts[1] && parts.length === 2) {
    return { page: "workspace", workspaceID: parts[1] };
  }
  if (parts.length === 1 && parts[0] === "agents") return { page: "agents" };
  if (parts.length === 1 && parts[0] === "environments") return { page: "environments" };
  return { page: "workspaces" };
}
