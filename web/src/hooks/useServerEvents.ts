import { useEffect, useRef, useState } from "react";

export interface ServerEvent {
  type: "connected" | "session.changed";
  workspaceID?: string;
  sessionID?: string;
  runID?: string;
  sequence: number;
}

interface SessionChangedPayload {
  workspace_id?: string;
  session_id?: string;
  run_id?: string;
}

export function useServerEvents(
  activeWorkspaceID: string,
  onWorkspaceChanged: (workspaceID: string) => void,
): ServerEvent | undefined {
  const [latest, setLatest] = useState<ServerEvent>();
  const workspaceRef = useRef(activeWorkspaceID);
  const changedRef = useRef(onWorkspaceChanged);
  workspaceRef.current = activeWorkspaceID;
  changedRef.current = onWorkspaceChanged;

  useEffect(() => {
    if (typeof EventSource === "undefined") return;
    const source = new EventSource("/api/events");
    let sequence = 0;
    source.addEventListener("connected", () => {
      if (workspaceRef.current) changedRef.current(workspaceRef.current);
      setLatest({ type: "connected", sequence: ++sequence });
    });
    source.addEventListener("session.changed", (raw) => {
      let payload: SessionChangedPayload;
      try {
        payload = JSON.parse((raw as MessageEvent<string>).data) as SessionChangedPayload;
      } catch {
        return;
      }
      if (payload.workspace_id) changedRef.current(payload.workspace_id);
      setLatest({
        type: "session.changed",
        workspaceID: payload.workspace_id,
        sessionID: payload.session_id,
        runID: payload.run_id,
        sequence: ++sequence,
      });
    });
    return () => source.close();
  }, []);

  return latest;
}
