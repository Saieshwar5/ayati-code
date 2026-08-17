import type {
  AgentDefinition,
  AgentInput,
  Branch,
  AuthorityChangeInput,
  Changes,
  CreateNewProjectInput,
  CreateWorkspaceInput,
  EnvironmentInput,
  EnvironmentVariable,
  Message,
  PublishInput,
  ProviderDefinition,
  ProviderConnectionInput,
  ProviderModel,
  Repository,
  SessionResponse,
  SkillDefinition,
  SkillInput,
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
  archivedWorkspaces: () => request<Workspace[]>("/api/workspaces?archived=true"),
  agents: () => request<AgentDefinition[]>("/api/agents"),
  archivedAgents: () => request<AgentDefinition[]>("/api/agents?archived=true"),
  agent: (id: string) => request<AgentDefinition>(`/api/agents/${id}`),
  providers: () => request<ProviderDefinition[]>("/api/providers"),
  providerModels: (id: string) =>
    request<ProviderModel[]>(`/api/providers/${encodeURIComponent(id)}/models`),
  configureProvider: (id: string, input: ProviderConnectionInput) =>
    request<ProviderDefinition>(`/api/providers/${encodeURIComponent(id)}`, {
      method: "PUT", body: JSON.stringify(input),
    }),
  testProvider: (id: string, input: ProviderConnectionInput) =>
    request<{ verified: boolean }>(`/api/providers/${encodeURIComponent(id)}/test`, {
      method: "POST", body: JSON.stringify(input),
    }),
  removeProvider: (id: string) =>
    request<void>(`/api/providers/${encodeURIComponent(id)}`, { method: "DELETE" }),
  createAgent: (input: AgentInput) =>
    request<AgentDefinition>("/api/agents", { method: "POST", body: JSON.stringify(input) }),
  updateAgent: (id: string, input: AgentInput) =>
    request<AgentDefinition>(`/api/agents/${id}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  setDefaultAgent: (id: string) =>
    request<AgentDefinition>(`/api/agents/${id}/default`, { method: "POST" }),
  duplicateAgent: (id: string) =>
    request<AgentDefinition>(`/api/agents/${id}/duplicate`, { method: "POST" }),
  archiveAgent: (id: string) =>
    request<void>(`/api/agents/${id}/archive`, { method: "POST" }),
  restoreAgent: (id: string) =>
    request<AgentDefinition>(`/api/agents/${id}/restore`, { method: "POST" }),
  skills: () => request<SkillDefinition[]>("/api/skills"),
  archivedSkills: () => request<SkillDefinition[]>("/api/skills?archived=true"),
  createSkill: (input: SkillInput) =>
    request<SkillDefinition>("/api/skills", { method: "POST", body: JSON.stringify(input) }),
  updateSkill: (id: string, input: SkillInput) =>
    request<SkillDefinition>(`/api/skills/${id}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  archiveSkill: (id: string) =>
    request<void>(`/api/skills/${id}/archive`, { method: "POST" }),
  restoreSkill: (id: string) =>
    request<SkillDefinition>(`/api/skills/${id}/restore`, { method: "POST" }),
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
  changeWorkspaceAuthority: (id: string, input: AuthorityChangeInput) =>
    request<Workspace>(`/api/workspaces/${id}/authority`, {
      method: "POST",
      body: JSON.stringify(input),
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
  selectSessionAgent: (workspaceID: string, sessionID: string, agentID: string) =>
    request<WorkspaceSession>(`/api/workspaces/${workspaceID}/sessions/${sessionID}`, {
      method: "PATCH",
      body: JSON.stringify({ agent_id: agentID }),
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
  publish: (workspaceID: string, input: PublishInput) =>
    request<Workspace>(`/api/workspaces/${workspaceID}/publish`, {
      method: "POST",
      body: JSON.stringify(input),
    }),
};
