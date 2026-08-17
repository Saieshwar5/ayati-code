import { useCallback, useEffect, useState } from "react";
import type {
  AuthorityChangeInput,
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
    const title = window.prompt("Rename session", session.title)?.trim();
    if (!title || title === session.title) return;
    const updated = await api.renameSession(workspaceID, session.id, title);
    setSessions((current) => ({
      ...current,
      [workspaceID]: (current[workspaceID] || []).map((item) =>
        item.id === updated.id ? updated : item),
    }));
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
    } catch (error) {
      window.alert((error as Error).message);
    }
  }, [refreshWorkspaces]);

  const configureProjectRoot = useCallback(async (workspaceID: string, projectRoot: string) => {
    await api.configureWorkspace(workspaceID, projectRoot);
    await refreshWorkspaces();
  }, [refreshWorkspaces]);

  const changeWorkspaceAuthority = useCallback(async (
    workspaceID: string,
    input: AuthorityChangeInput,
  ) => {
    const updated = await api.changeWorkspaceAuthority(workspaceID, input);
    updateWorkspaceIn(setWorkspaces, updated);
  }, []);

  const archiveWorkspace = useCallback(async (workspace: Workspace) => {
    if (!window.confirm(`Archive “${repositoryName(workspace.repository)}”?\n\nIts repository, sessions, and history will be preserved.`)) return false;
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
    const confirmed = window.confirm(
      `Delete workspace “${repositoryName(workspace.repository)}”?\n\nThis permanently removes its local clone, sessions, and history. The GitHub repository is not deleted.`,
    );
    if (!confirmed) return false;
    try {
      await api.deleteWorkspace(workspace.id);
      await refreshWorkspaces();
      return true;
    } catch (error) {
      window.alert((error as Error).message);
      return false;
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
    workspaces, archivedWorkspaces, sessions, loading, loadError,
    createWorkspace, createNewProject, createSession, renameSession, deleteSession,
    workspaceAction, configureProjectRoot, changeWorkspaceAuthority,
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
