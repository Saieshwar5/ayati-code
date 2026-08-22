import { useEffect, useRef } from "react";

// useRunEvents subscribes to run.changed SSE notices for a workspace. Events
// carry only IDs and state; the component reloads authoritative state from the
// API after each notice.
export function useRunEvents(workspaceID: string, onRunChange: () => void): void {
  const callback = useRef(onRunChange);
  callback.current = onRunChange;

  useEffect(() => {
    if (typeof window === "undefined" || !("EventSource" in window)) {
      return;
    }
    const source = new EventSource("/api/events");
    source.addEventListener("run.changed", (event) => {
      try {
        const data = JSON.parse((event as MessageEvent).data) as {
          workspace_id?: string;
        };
        if (!data.workspace_id || data.workspace_id === workspaceID) {
          callback.current();
        }
      } catch {
        // Ignore malformed notices; the next event or manual action reloads.
      }
    });
    return () => source.close();
  }, [workspaceID]);
}
