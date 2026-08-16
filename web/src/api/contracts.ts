export type WorkspaceStatus =
  | "creating"
  | "initializing"
  | "initialization_failed"
  | "ready"
  | "stopped";

export type SessionStatus = "idle" | "working" | "review" | "failed";
export type WorkspaceAuthority = "explore" | "develop";

export interface User {
  id: number;
  login: string;
  avatar_url: string;
}

export interface SessionResponse {
  github_configured: boolean;
  authenticated: boolean;
  user?: User;
}

export interface Repository {
  id: number;
  full_name: string;
  clone_url: string;
  default_branch: string;
  private: boolean;
}

export interface Branch {
  name: string;
  commit: { sha: string };
}

export interface Workspace {
  id: string;
  repository: string;
  clone_url: string;
  base_branch: string;
  branch: string;
  create_branch: boolean;
  authority: WorkspaceAuthority;
  effective_mount_mode?: "ro" | "rw";
  setup_command: string;
  path: string;
  sandbox_name: string;
  status: WorkspaceStatus;
  error?: string;
  pull_request_number?: number;
  pull_request_url?: string;
  created_at: string;
  updated_at: string;
}

export interface WorkspaceSession {
  id: string;
  workspace_id: string;
  title: string;
  status: SessionStatus;
  error?: string;
  created_at: string;
  updated_at: string;
}

export interface ToolCall {
  id: string;
  type: string;
  function: { name: string; arguments: string };
}

export interface Message {
  role: "user" | "assistant" | "tool" | string;
  content?: string;
  tool_calls?: ToolCall[];
  tool_call_id?: string;
}

export interface Changes {
  status: string;
  diff: string;
}

export interface EnvironmentInput {
  name: string;
  value: string;
  expose_during_setup: boolean;
}

export interface EnvironmentVariable {
  name: string;
  configured: boolean;
  expose_during_setup: boolean;
  updated_at: string;
}

export interface CreateWorkspaceInput {
  repository: string;
  base_branch: string;
  branch: string;
  create_branch: boolean;
  authority: WorkspaceAuthority;
  setup_command: string;
  environment: EnvironmentInput[];
}

export interface PublishInput {
  commit_message: string;
  title: string;
  body: string;
}
