import type {
  AgentRun,
  Branch,
  Changes,
  CreateNewProjectInput,
  CreateWorkspaceInput,
  EnvironmentInput,
  EnvironmentVariable,
  Message,
  PublishInput,
  Repository,
  SessionResponse,
  Workspace,
  WorkspaceSession,
} from "./contracts";
import type { ComputeEnvironment, CreateComputeEnvironmentInput } from "./environment-contracts";

export class ApiError extends Error {
  constructor(message: string, readonly status: number) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body) headers.set("Content-Type", "application/json");
  if (init.method && init.method !== "GET") headers.set("X-Perpetual-Request", "1");
  const response = await fetch(path, { ...init, headers });
  if (response.status === 204) return undefined as T;
  const body = (await response.json().catch(() => ({}))) as { error?: string };
  if (!response.ok) {
    throw new ApiError(body.error || `Request failed (${response.status})`, response.status);
  }
  return body as T;
}

export const api = {
  session: () => request<SessionResponse>("/api/session"),
  logout: () => request<void>("/api/logout", { method: "POST" }),
  repositories: () => request<Repository[]>("/api/repositories"),
  branches: (repository: string) =>
    request<Branch[]>(`/api/repositories/${repository}/branches`),
  workspaces: () => request<Workspace[]>("/api/workspaces"),
  archivedWorkspaces: () => request<Workspace[]>("/api/workspaces?archived=true"),
  createWorkspace: (input: CreateWorkspaceInput) =>
    request<Workspace>("/api/workspaces", { method: "POST", body: JSON.stringify(input) }),
  createNewProject: (input: CreateNewProjectInput) =>
    request<Workspace>("/api/workspaces/new-project", {
      method: "POST",
      body: JSON.stringify(input),
    }),
  initializeWorkspace: (id: string) =>
    request<void>(`/api/workspaces/${id}/initialize`, { method: "POST" }),
  configureWorkspace: (id: string, projectRoot: string) =>
    request<void>(`/api/workspaces/${id}/configure`, {
      method: "POST",
      body: JSON.stringify({ project_root: projectRoot }),
    }),
  stopWorkspace: (id: string) =>
    request<void>(`/api/workspaces/${id}/stop`, { method: "POST" }),
  resumeWorkspace: (id: string) =>
    request<void>(`/api/workspaces/${id}/resume`, { method: "POST" }),
  archiveWorkspace: (id: string) =>
    request<void>(`/api/workspaces/${id}/archive`, { method: "POST" }),
  restoreWorkspace: (id: string) =>
    request<void>(`/api/workspaces/${id}/restore`, { method: "POST" }),
  deleteWorkspace: (id: string) =>
    request<void>(`/api/workspaces/${id}`, { method: "DELETE" }),
  sessions: (workspaceID: string) =>
    request<WorkspaceSession[]>(`/api/workspaces/${workspaceID}/sessions`),
  sessionByID: (workspaceID: string, sessionID: string) =>
    request<WorkspaceSession>(`/api/workspaces/${workspaceID}/sessions/${sessionID}`),
  createSession: (workspaceID: string) =>
    request<WorkspaceSession>(`/api/workspaces/${workspaceID}/sessions`, {
      method: "POST",
      body: JSON.stringify({}),
    }),
  renameSession: (workspaceID: string, sessionID: string, title: string) =>
    request<WorkspaceSession>(`/api/workspaces/${workspaceID}/sessions/${sessionID}`, {
      method: "PATCH",
      body: JSON.stringify({ title }),
    }),
  deleteSession: (workspaceID: string, sessionID: string) =>
    request<void>(`/api/workspaces/${workspaceID}/sessions/${sessionID}`, { method: "DELETE" }),
  messages: (workspaceID: string, sessionID: string) =>
    request<Message[]>(`/api/workspaces/${workspaceID}/sessions/${sessionID}/messages`),
  sendMessage: (workspaceID: string, sessionID: string, text: string) =>
    request<AgentRun>(`/api/workspaces/${workspaceID}/sessions/${sessionID}/messages`, {
      method: "POST",
      body: JSON.stringify({ text }),
    }),
  cancelRun: (workspaceID: string, sessionID: string, runID: string) =>
    request<void>(`/api/workspaces/${workspaceID}/sessions/${sessionID}/runs/${runID}/cancel`, {
      method: "POST",
    }),
  changes: (workspaceID: string) =>
    request<Changes>(`/api/workspaces/${workspaceID}/changes`),
  environment: (workspaceID: string) =>
    request<EnvironmentVariable[]>(`/api/workspaces/${workspaceID}/environment`),
  upsertEnvironment: (workspaceID: string, input: EnvironmentInput) =>
    request<EnvironmentVariable>(`/api/workspaces/${workspaceID}/environment`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
  deleteEnvironment: (workspaceID: string, name: string) =>
    request<void>(`/api/workspaces/${workspaceID}/environment/${encodeURIComponent(name)}`, {
      method: "DELETE",
    }),
  environments: () => request<ComputeEnvironment[]>("/api/environments"),
  createEnvironmentCapacity: (input: CreateComputeEnvironmentInput) =>
    request<ComputeEnvironment>("/api/environments", {
      method: "POST", body: JSON.stringify(input),
    }),
  repairEnvironmentCapacity: (id: string) =>
    request<ComputeEnvironment>(`/api/environments/${encodeURIComponent(id)}/repair`, {
      method: "POST",
    }),
  deleteEnvironmentCapacity: (id: string) =>
    request<void>(`/api/environments/${encodeURIComponent(id)}`, { method: "DELETE" }),
  publish: (workspaceID: string, input: PublishInput) =>
    request<Workspace>(`/api/workspaces/${workspaceID}/publish`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
};
