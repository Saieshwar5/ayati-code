import { useCallback, useEffect, useRef, useState } from "react";
import type {
  Message,
  PublishInput,
  Workspace,
  WorkspaceSession,
} from "../api/contracts";
import { api } from "../api/client";
import { statusLabel } from "../app/format";

interface UseWorkspaceDetailOptions {
  workspace?: Workspace;
  session?: WorkspaceSession;
  onSessionUpdate: (session: WorkspaceSession) => void;
  onWorkspaceUpdate: (workspace: Workspace) => void;
  onWorkspacesRefresh: () => Promise<Workspace[]>;
}

export function useWorkspaceDetail(options: UseWorkspaceDetailOptions) {
  const { workspace, session } = options;
  const [messages, setMessages] = useState<Message[]>([]);
  const [messageError, setMessageError] = useState("");
  const [changes, setChanges] = useState("No changes loaded.");
  const [sending, setSending] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const selection = useRef("");
  const pollBusy = useRef(false);

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
    if (!workspace || !session || pollBusy.current) return;
    pollBusy.current = true;
    const key = `${workspace.id}:${session.id}`;
    try {
      const [nextMessages, nextSession] = await Promise.all([
        api.messages(workspace.id, session.id),
        api.sessionByID(workspace.id, session.id),
        options.onWorkspacesRefresh(),
      ]);
      options.onSessionUpdate(nextSession);
      if (selection.current === key) setMessages(nextMessages);
    } catch (error) {
      if (selection.current === key) setMessageError((error as Error).message);
    } finally {
      pollBusy.current = false;
    }
  }, [options, session, workspace]);

  const sendMessage = useCallback(
    async (text: string) => {
      if (!workspace || !session || !text.trim()) return false;
      const workspaceID = workspace.id;
      const sessionID = session.id;
      setMessageError("");
      setSending(true);
      const timer = window.setInterval(() => void refreshRun(), 800);
      let sent = false;
      try {
        await api.sendMessage(workspaceID, sessionID, text.trim());
        sent = true;
      } catch (error) {
        setMessageError((error as Error).message);
      } finally {
        window.clearInterval(timer);
        await refreshRun();
        await loadChanges();
        setSending(false);
      }
      return sent;
    },
    [loadChanges, refreshRun, session, workspace],
  );

  const publish = useCallback(
    async (input: PublishInput) => {
      if (!workspace) return false;
      setPublishing(true);
      try {
        const updated = await api.publish(workspace.id, input);
        options.onWorkspaceUpdate(updated);
        await loadChanges();
        return true;
      } finally {
        setPublishing(false);
      }
    },
    [loadChanges, options, workspace],
  );

  return {
    messages,
    messageError,
    changes,
    sending,
    publishing,
    loadChanges,
    sendMessage,
    publish,
  };
}
