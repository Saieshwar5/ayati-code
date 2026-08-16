import { useEffect, useState } from "react";
import type { SessionResponse } from "../api/contracts";
import { api } from "../api/client";
import { ConfigureView, ErrorView, LoadingView, LoginView } from "../auth/EntryViews";
import { WorkspaceApplication } from "./WorkspaceApplication";

type LoadState =
  | { kind: "loading" }
  | { kind: "error"; message: string }
  | { kind: "ready"; session: SessionResponse };

export function App() {
  const [state, setState] = useState<LoadState>({ kind: "loading" });

  useEffect(() => {
    let current = true;
    api.session().then(
      (session) => current && setState({ kind: "ready", session }),
      (error: Error) => current && setState({ kind: "error", message: error.message }),
    );
    return () => {
      current = false;
    };
  }, []);

  if (state.kind === "loading") return <LoadingView />;
  if (state.kind === "error") return <ErrorView message={state.message} />;
  if (!state.session.github_configured) return <ConfigureView />;
  if (!state.session.authenticated || !state.session.user) return <LoginView />;
  return <WorkspaceApplication user={state.session.user} />;
}
