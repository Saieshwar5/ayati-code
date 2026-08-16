import { useCallback, useEffect, useMemo, useState } from "react";
import type {
  CreateWorkspaceInput,
  Repository,
  User,
  Workspace,
  WorkspaceSession,
} from "../api/contracts";
import { api } from "../api/client";
import { repositoryName } from "./format";

const refreshingStatuses = new Set(["creating", "initializing"]);
export type MainView = "empty" | "create" | "workspace";

export function useWorkspaceController(user: User) {
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [repositoryError, setRepositoryError] = useState("");
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [sessions, setSessions] = useState<Record<string, WorkspaceSession[]>>({});
  const [activeWorkspaceID, setActiveWorkspaceID] = useState("");
  const [activeSessionID, setActiveSessionID] = useState("");
  const [expandedWorkspaceID, setExpandedWorkspaceID] = useState("");
  const [view, setView] = useState<MainView>("empty");
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");

  const loadSessions = useCallback(async (workspaceID: string) => {
    const values = await api.sessions(workspaceID);
    setSessions((current) => ({ ...current, [workspaceID]: values }));
    return values;
  }, []);

  const openWorkspace = useCallback(
    async (workspaceID: string, preferredSessionID = "") => {
      const values = await loadSessions(workspaceID);
      const sessionID = preferredSessionID || values[0]?.id || "";
      setExpandedWorkspaceID(workspaceID);
      setActiveWorkspaceID(workspaceID);
      setActiveSessionID(sessionID);
      setView(sessionID ? "workspace" : "empty");
    },
    [loadSessions],
  );

  useEffect(() => {
    let current = true;
    Promise.allSettled([api.repositories(), api.workspaces()]).then(async ([repos, spaces]) => {
      if (!current) return;
      if (repos.status === "fulfilled") setRepositories(repos.value);
      else setRepositoryError(repos.reason instanceof Error ? repos.reason.message : "Repositories unavailable");
      if (spaces.status === "rejected") {
        setLoadError(spaces.reason instanceof Error ? spaces.reason.message : "Workspaces unavailable");
        setLoading(false);
        return;
      }
      setWorkspaces(spaces.value);
      setLoading(false);
      const first = spaces.value[0];
      if (first) await openWorkspace(first.id).catch((error: Error) => setLoadError(error.message));
    });
    return () => {
      current = false;
    };
  }, [openWorkspace]);

  useEffect(() => {
    if (!workspaces.some((workspace) => refreshingStatuses.has(workspace.status))) return;
    const timer = window.setTimeout(() => {
      api.workspaces().then(setWorkspaces, (error: Error) => setLoadError(error.message));
    }, 1200);
    return () => window.clearTimeout(timer);
  }, [workspaces]);

  const activeWorkspace = useMemo(
    () => workspaces.find((workspace) => workspace.id === activeWorkspaceID),
    [activeWorkspaceID, workspaces],
  );
  const activeSession = useMemo(
    () => (sessions[activeWorkspaceID] || []).find((session) => session.id === activeSessionID),
    [activeSessionID, activeWorkspaceID, sessions],
  );

  const refreshWorkspaces = useCallback(async () => {
    const values = await api.workspaces();
    setWorkspaces(values);
    return values;
  }, []);

  const toggleWorkspace = useCallback(
    async (workspaceID: string) => {
      if (expandedWorkspaceID === workspaceID) {
        setExpandedWorkspaceID("");
        return;
      }
      await openWorkspace(workspaceID);
    },
    [expandedWorkspaceID, openWorkspace],
  );

  const showCreate = useCallback(() => {
    setActiveWorkspaceID("");
    setActiveSessionID("");
    setView("create");
  }, []);

  const closeCreate = useCallback(() => setView("empty"), []);

  const createWorkspace = useCallback(
    async (input: CreateWorkspaceInput) => {
      const created = await api.createWorkspace(input);
      const values = await refreshWorkspaces();
      if (values.some((workspace) => workspace.id === created.id)) await openWorkspace(created.id);
    },
    [openWorkspace, refreshWorkspaces],
  );

  const createSession = useCallback(
    async (workspaceID: string) => {
      const created = await api.createSession(workspaceID);
      await loadSessions(workspaceID);
      await openWorkspace(workspaceID, created.id);
    },
    [loadSessions, openWorkspace],
  );

  const renameSession = useCallback(
    async (workspaceID: string, session: WorkspaceSession) => {
      const title = window.prompt("Rename session", session.title)?.trim();
      if (!title || title === session.title) return;
      const updated = await api.renameSession(workspaceID, session.id, title);
      setSessions((current) => ({
        ...current,
        [workspaceID]: (current[workspaceID] || []).map((item) =>
          item.id === updated.id ? updated : item,
        ),
      }));
    },
    [],
  );

  const deleteSession = useCallback(
    async (workspaceID: string, session: WorkspaceSession) => {
      const confirmed = window.confirm(
        `Delete “${session.title}”?\n\nThis removes its conversation and activity history. Workspace files and changes are not reverted.`,
      );
      if (!confirmed) return;
      try {
        await api.deleteSession(workspaceID, session.id);
        const values = await loadSessions(workspaceID);
        if (activeSessionID === session.id) {
          setActiveSessionID(values[0]?.id || "");
          if (!values.length) setView("empty");
        }
      } catch (error) {
        window.alert((error as Error).message);
      }
    },
    [activeSessionID, loadSessions],
  );

  const workspaceAction = useCallback(
    async (workspaceID: string, action: "initialize" | "stop") => {
      try {
        if (action === "initialize") {
          await api.initializeWorkspace(workspaceID);
          setWorkspaces((current) => current.map((workspace) =>
            workspace.id === workspaceID
              ? { ...workspace, status: "initializing", preparation_stage: "pending", error: undefined }
              : workspace,
          ));
        } else {
          await api.stopWorkspace(workspaceID);
          await refreshWorkspaces();
        }
      } catch (error) {
        window.alert((error as Error).message);
      }
    },
    [refreshWorkspaces],
  );

  const configureProjectRoot = useCallback(
    async (workspaceID: string, projectRoot: string) => {
      await api.configureWorkspace(workspaceID, projectRoot);
      await refreshWorkspaces();
    },
    [refreshWorkspaces],
  );

  const deleteWorkspace = useCallback(
    async (workspace: Workspace) => {
      const confirmed = window.confirm(
        `Delete workspace “${repositoryName(workspace.repository)}”?\n\nThis permanently removes its local clone, every session, and all conversation and activity history. The GitHub branch and pull request are not deleted.`,
      );
      if (!confirmed) return;
      try {
        await api.deleteWorkspace(workspace.id);
        setSessions((current) => {
          const next = { ...current };
          delete next[workspace.id];
          return next;
        });
        const values = await refreshWorkspaces();
        if (activeWorkspaceID === workspace.id) {
          setActiveWorkspaceID("");
          setActiveSessionID("");
          setExpandedWorkspaceID("");
          setView("empty");
          if (values[0]) await openWorkspace(values[0].id);
        }
      } catch (error) {
        window.alert((error as Error).message);
      }
    },
    [activeWorkspaceID, openWorkspace, refreshWorkspaces],
  );

  const updateSession = useCallback((updated: WorkspaceSession) => {
    setSessions((current) => {
      const values = current[updated.workspace_id] || [];
      const next = values.map((item) => (item.id === updated.id ? updated : item));
      next.sort((left, right) => Date.parse(right.updated_at) - Date.parse(left.updated_at));
      return { ...current, [updated.workspace_id]: next };
    });
  }, []);

  const updateWorkspace = useCallback((updated: Workspace) => {
    setWorkspaces((current) => current.map((item) => (item.id === updated.id ? updated : item)));
  }, []);

  const logout = useCallback(async () => {
    await api.logout();
    window.location.reload();
  }, []);

  return {
    user,
    repositories,
    repositoryError,
    workspaces,
    sessions,
    activeWorkspace,
    activeSession,
    expandedWorkspaceID,
    view,
    loading,
    loadError,
    openWorkspace,
    toggleWorkspace,
    showCreate,
    closeCreate,
    createWorkspace,
    createSession,
    renameSession,
    deleteSession,
    workspaceAction,
    configureProjectRoot,
    deleteWorkspace,
    refreshWorkspaces,
    loadSessions,
    updateSession,
    updateWorkspace,
    logout,
  };
}

export type WorkspaceController = ReturnType<typeof useWorkspaceController>;
