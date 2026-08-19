import { useCallback, useEffect, useRef, useState } from "react";
import type {
  CreateNewProjectInput,
  CreateWorkspaceInput,
  Repository,
  User,
  Workspace,
  WorkspaceSession,
} from "../api/contracts";
import { api, ApiError } from "../api/client";
import { repositoryName } from "./format";

const refreshingStatuses = new Set(["creating", "initializing"]);

export function useWorkspaceController(user: User) {
  const [repositories, setRepositories] = useState<Repository[]>([]);
  const [repositoryError, setRepositoryError] = useState("");
  const [repositoryReconnectRequired, setRepositoryReconnectRequired] = useState(false);
  const [workspaces, setWorkspaces] = useState<Workspace[]>([]);
  const [archivedWorkspaces, setArchivedWorkspaces] = useState<Workspace[]>([]);
  const [sessions, setSessions] = useState<Record<string, WorkspaceSession[]>>({});
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState("");
  const [deletingWorkspaceIDs, setDeletingWorkspaceIDs] = useState<ReadonlySet<string>>(new Set());
  const deletingWorkspaceIDsRef = useRef(new Set<string>());

  const refreshWorkspaces = useCallback(async () => {
    const [active, archived] = await Promise.all([api.workspaces(), api.archivedWorkspaces()]);
    setWorkspaces(active);
    setArchivedWorkspaces(archived);
    return active;
  }, []);

  useEffect(() => {
    let current = true;
    Promise.allSettled([api.repositories(), api.workspaces(), api.archivedWorkspaces()]).then(
      ([repos, active, archived]) => {
        if (!current) return;
        if (repos.status === "fulfilled") setRepositories(repos.value);
        else {
          const reason = repos.reason;
          setRepositoryError(reason instanceof Error ? reason.message : "Repositories unavailable");
          setRepositoryReconnectRequired(reason instanceof ApiError && reason.status === 401);
        }
        if (active.status === "fulfilled") setWorkspaces(active.value);
        else setLoadError(active.reason instanceof Error ? active.reason.message : "Workspaces unavailable");
        if (archived.status === "fulfilled") setArchivedWorkspaces(archived.value);
        setLoading(false);
      },
    );
    return () => {
      current = false;
    };
  }, []);

  useEffect(() => {
    if (!workspaces.some((workspace) => refreshingStatuses.has(workspace.status))) return;
    const timer = window.setTimeout(() => {
      void refreshWorkspaces().catch((error: Error) => setLoadError(error.message));
    }, 1200);
    return () => window.clearTimeout(timer);
  }, [refreshWorkspaces, workspaces]);

  const loadSessions = useCallback(async (workspaceID: string) => {
    const values = await api.sessions(workspaceID);
    setSessions((current) => ({ ...current, [workspaceID]: values }));
    return values;
  }, []);

  const createWorkspace = useCallback(async (input: CreateWorkspaceInput) => {
    const created = await api.createWorkspace(input);
    await refreshWorkspaces();
    return created;
  }, [refreshWorkspaces]);

  const createNewProject = useCallback(async (input: CreateNewProjectInput) => {
    const created = await api.createNewProject(input);
    await refreshWorkspaces();
    return created;
  }, [refreshWorkspaces]);

  const createSession = useCallback(async (workspaceID: string) => {
    const created = await api.createSession(workspaceID);
    await loadSessions(workspaceID);
    return created;
  }, [loadSessions]);

  const renameSession = useCallback(async (workspaceID: string, session: WorkspaceSession) => {
    const title = window.prompt("Rename conversation", session.title)?.trim();
    if (!title || title === session.title) return;
    const updated = await api.renameSession(workspaceID, session.id, title);
    setSessions((current) => ({
      ...current,
      [workspaceID]: (current[workspaceID] || []).map((item) =>
        item.id === updated.id ? updated : item),
    }));
  }, []);

  const selectSessionAgent = useCallback(async (
    workspaceID: string,
    sessionID: string,
    agentID: string,
  ) => {
    const updated = await api.selectSessionAgent(workspaceID, sessionID, agentID);
    setSessions((current) => ({
      ...current,
      [workspaceID]: (current[workspaceID] || []).map((item) =>
        item.id === updated.id ? updated : item),
    }));
    return updated;
  }, []);

  const deleteSession = useCallback(async (workspaceID: string, session: WorkspaceSession) => {
    const confirmed = window.confirm(
      `Delete “${session.title}”?\n\nThis removes its conversation and activity history. Workspace files and changes are not reverted.`,
    );
    if (!confirmed) return false;
    try {
      await api.deleteSession(workspaceID, session.id);
      await loadSessions(workspaceID);
      return true;
    } catch (error) {
      window.alert((error as Error).message);
      return false;
    }
  }, [loadSessions]);

  const workspaceAction = useCallback(async (
    workspaceID: string,
    action: "initialize" | "resume" | "stop",
  ) => {
    try {
      if (action === "initialize") await api.initializeWorkspace(workspaceID);
      if (action === "resume") await api.resumeWorkspace(workspaceID);
      if (action === "stop") await api.stopWorkspace(workspaceID);
      await refreshWorkspaces();
      return true;
    } catch (error) {
      window.alert((error as Error).message);
      return false;
    }
  }, [refreshWorkspaces]);

  const configureProjectRoot = useCallback(async (workspaceID: string, projectRoot: string) => {
    await api.configureWorkspace(workspaceID, projectRoot);
    await refreshWorkspaces();
  }, [refreshWorkspaces]);

  const archiveWorkspace = useCallback(async (workspace: Workspace) => {
    if (!window.confirm(`Archive “${repositoryName(workspace.repository)}”?\n\nIts repository and conversation history will be preserved.`)) return false;
    try {
      await api.archiveWorkspace(workspace.id);
      await refreshWorkspaces();
      return true;
    } catch (error) {
      window.alert((error as Error).message);
      return false;
    }
  }, [refreshWorkspaces]);

  const restoreWorkspace = useCallback(async (workspaceID: string) => {
    await api.restoreWorkspace(workspaceID);
    await refreshWorkspaces();
  }, [refreshWorkspaces]);

  const deleteWorkspace = useCallback(async (workspace: Workspace) => {
    if (deletingWorkspaceIDsRef.current.has(workspace.id)) return false;
    const confirmed = window.confirm(
      `Permanently delete local workspace “${repositoryName(workspace.repository)}”?\n\nThis removes its local clone, cache, conversations, and unpublished changes. The GitHub repository, remote branches, and pull requests will not be changed.`,
    );
    if (!confirmed) return false;
    deletingWorkspaceIDsRef.current.add(workspace.id);
    setDeletingWorkspaceIDs(new Set(deletingWorkspaceIDsRef.current));
    try {
      await api.deleteWorkspace(workspace.id);
      await refreshWorkspaces();
      return true;
    } catch (error) {
      await refreshWorkspaces().catch(() => undefined);
      window.alert((error as Error).message);
      return false;
    } finally {
      deletingWorkspaceIDsRef.current.delete(workspace.id);
      setDeletingWorkspaceIDs(new Set(deletingWorkspaceIDsRef.current));
    }
  }, [refreshWorkspaces]);

  const updateSession = useCallback((updated: WorkspaceSession) => {
    setSessions((current) => {
      const next = (current[updated.workspace_id] || []).map((item) =>
        item.id === updated.id ? updated : item);
      next.sort((left, right) => Date.parse(right.updated_at) - Date.parse(left.updated_at));
      return { ...current, [updated.workspace_id]: next };
    });
  }, []);

  const updateWorkspace = useCallback((updated: Workspace) => {
    updateWorkspaceIn(setWorkspaces, updated);
  }, []);

  const logout = useCallback(async () => {
    await api.logout();
    window.location.reload();
  }, []);

  return {
    user, repositories, repositoryError, repositoryReconnectRequired,
    workspaces, archivedWorkspaces, sessions, loading, loadError, deletingWorkspaceIDs,
    createWorkspace, createNewProject, createSession, renameSession, selectSessionAgent, deleteSession,
    workspaceAction, configureProjectRoot,
    archiveWorkspace, restoreWorkspace, deleteWorkspace,
    refreshWorkspaces, loadSessions, updateSession, updateWorkspace, logout,
  };
}

function updateWorkspaceIn(
  setter: React.Dispatch<React.SetStateAction<Workspace[]>>,
  updated: Workspace,
) {
  setter((current) => current.map((item) => (item.id === updated.id ? updated : item)));
}

export type WorkspaceController = ReturnType<typeof useWorkspaceController>;
