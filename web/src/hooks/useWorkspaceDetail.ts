import { useCallback, useEffect, useRef, useState } from "react";
import type {
  Message,
  PublishInput,
  Workspace,
  WorkspaceSession,
} from "../api/contracts";
import { api } from "../api/client";
import { statusLabel } from "../app/format";
import type { ServerEvent } from "./useServerEvents";

interface UseWorkspaceDetailOptions {
  workspace?: Workspace;
  session?: WorkspaceSession;
  onSessionUpdate: (session: WorkspaceSession) => void;
  onWorkspaceUpdate: (workspace: Workspace) => void;
  serverEvent?: ServerEvent;
}

export function useWorkspaceDetail(options: UseWorkspaceDetailOptions) {
  const {
    workspace, session, onSessionUpdate, onWorkspaceUpdate, serverEvent,
  } = options;
  const workspaceID = workspace?.id ?? "";
  const sessionID = session?.id ?? "";
  const [messages, setMessages] = useState<Message[]>([]);
  const [messageError, setMessageError] = useState("");
  const [changes, setChanges] = useState("No changes loaded.");
  const [sendingSessionID, setSendingSessionID] = useState("");
  const [stoppingSessionID, setStoppingSessionID] = useState("");
  const [publishing, setPublishing] = useState(false);
  const selection = useRef("");
  const refreshBusy = useRef(false);
  const refreshPending = useRef(false);
  const stopRequested = useRef("");

  const loadMessages = useCallback(async () => {
    if (!workspace || !session) return;
    const key = `${workspace.id}:${session.id}`;
    try {
      const values = await api.messages(workspace.id, session.id);
      if (selection.current === key) setMessages(values);
    } catch (error) {
      if (selection.current === key) setMessageError((error as Error).message);
    }
  }, [session, workspace]);

  const loadChanges = useCallback(async () => {
    if (!workspace) return;
    const workspaceID = workspace.id;
    if (workspace.status !== "ready") {
      setChanges(
        `Changes are available after the workspace is ready.\n\nCurrent status: ${statusLabel(workspace.status)}`,
      );
      return;
    }
    setChanges("Loading changes…");
    try {
      const value = await api.changes(workspaceID);
      if (selection.current.startsWith(`${workspaceID}:`)) {
        setChanges([value.status, value.diff].filter(Boolean).join("\n") || "Working tree is clean.");
      }
    } catch (error) {
      if (selection.current.startsWith(`${workspaceID}:`)) setChanges((error as Error).message);
    }
  }, [workspace]);

  useEffect(() => {
    const key = workspace && session ? `${workspace.id}:${session.id}` : "";
    selection.current = key;
    setMessages([]);
    setMessageError("");
    if (!key) {
      setChanges("No changes loaded.");
      return;
    }
    void Promise.all([loadMessages(), loadChanges()]);
  }, [loadChanges, loadMessages, session, workspace]);

  const refreshRun = useCallback(async () => {
    if (!workspaceID || !sessionID) return;
    if (refreshBusy.current) {
      refreshPending.current = true;
      return;
    }
    refreshBusy.current = true;
    const key = `${workspaceID}:${sessionID}`;
    try {
      do {
        refreshPending.current = false;
        const [nextMessages, nextSession] = await Promise.all([
          api.messages(workspaceID, sessionID),
          api.sessionByID(workspaceID, sessionID),
        ]);
        onSessionUpdate(nextSession);
        if (selection.current === key) setMessages(nextMessages);
        if (nextSession.status !== "working") await loadChanges();
      } while (refreshPending.current && selection.current === key);
    } catch (error) {
      if (selection.current === key) setMessageError((error as Error).message);
    } finally {
      refreshBusy.current = false;
    }
  }, [loadChanges, onSessionUpdate, sessionID, workspaceID]);

  useEffect(() => {
    if (!serverEvent) return;
    if (serverEvent.type === "connected" ||
      (serverEvent.workspaceID === workspaceID && serverEvent.sessionID === sessionID)) {
      void refreshRun();
    }
  }, [refreshRun, serverEvent, sessionID, workspaceID]);

  const sendMessage = useCallback(
    async (text: string) => {
      if (!workspace || !session || !text.trim()) return false;
      const workspaceID = workspace.id;
      const sessionID = session.id;
      const key = `${workspaceID}:${sessionID}`;
      setMessageError("");
      stopRequested.current = "";
      setSendingSessionID(sessionID);
      let sent = false;
      try {
        await api.sendMessage(workspaceID, sessionID, text.trim());
        sent = true;
        await refreshRun();
      } catch (error) {
        if (selection.current === key && stopRequested.current !== key) {
          setMessageError((error as Error).message);
        }
      } finally {
        setSendingSessionID((current) => current === sessionID ? "" : current);
        if (stopRequested.current === key) stopRequested.current = "";
      }
      return sent;
    },
    [refreshRun, session, workspace],
  );

  const stopRun = useCallback(async () => {
    if (!workspace || !session || !session.active_run_id) return false;
    const workspaceID = workspace.id;
    const sessionID = session.id;
    const key = `${workspaceID}:${sessionID}`;
    stopRequested.current = key;
    setStoppingSessionID(sessionID);
    setMessageError("");
    try {
      await api.cancelRun(workspaceID, sessionID, session.active_run_id);
      await refreshRun();
      return true;
    } catch (error) {
      stopRequested.current = "";
      if (selection.current === key) setMessageError((error as Error).message);
      return false;
    } finally {
      setStoppingSessionID((current) => current === sessionID ? "" : current);
    }
  }, [refreshRun, session, workspace]);

  const publish = useCallback(
    async (input: PublishInput) => {
      if (!workspace) return false;
      setPublishing(true);
      try {
        const updated = await api.publish(workspace.id, input);
        onWorkspaceUpdate(updated);
        await loadChanges();
        return true;
      } finally {
        setPublishing(false);
      }
    },
    [loadChanges, onWorkspaceUpdate, workspace],
  );

  return {
    messages,
    messageError,
    changes,
    sending: Boolean(session && sendingSessionID === session.id),
    stopping: Boolean(session && stoppingSessionID === session.id),
    publishing,
    loadChanges,
    sendMessage,
    stopRun,
    publish,
  };
}
