import type {
  Branch,
  Changes,
  CreateWorkspaceInput,
  Message,
  PublishInput,
  Repository,
  SessionResponse,
  Workspace,
  WorkspaceSession,
} from "./contracts";

export class ApiError extends Error {
  constructor(message: string, readonly status: number) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body) headers.set("Content-Type", "application/json");
  if (init.method && init.method !== "GET") headers.set("X-Ayati-Request", "1");
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
  createWorkspace: (input: CreateWorkspaceInput) =>
    request<Workspace>("/api/workspaces", { method: "POST", body: JSON.stringify(input) }),
  initializeWorkspace: (id: string) =>
    request<void>(`/api/workspaces/${id}/initialize`, { method: "POST" }),
  stopWorkspace: (id: string) =>
    request<void>(`/api/workspaces/${id}/stop`, { method: "POST" }),
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
    request<unknown>(`/api/workspaces/${workspaceID}/sessions/${sessionID}/messages`, {
      method: "POST",
      body: JSON.stringify({ text }),
    }),
  changes: (workspaceID: string) =>
    request<Changes>(`/api/workspaces/${workspaceID}/changes`),
  publish: (workspaceID: string, input: PublishInput) =>
    request<Workspace>(`/api/workspaces/${workspaceID}/publish`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
};
