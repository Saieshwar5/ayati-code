export type WorkspaceStatus =
  | "creating"
  | "initializing"
  | "initialization_failed"
  | "needs_configuration"
  | "ready"
  | "stopped";

export type SessionStatus = "idle" | "working" | "review" | "failed";
export type WorkspaceAuthority = "explore" | "develop";
export type BranchMode = "new" | "existing" | "direct";
export type PreparationStage =
  | "pending"
  | "cloning"
  | "analyzing"
  | "installing"
  | "verifying"
  | "sealing"
  | "needs_configuration"
  | "ready"
  | "failed";

export interface ProjectCandidate {
  project_root: string;
  languages: string[];
  package_managers: string[];
}

export interface ProjectProfile {
  project_root: string;
  languages: string[];
  runtime_versions: string[];
  package_managers: string[];
  lockfiles: string[];
  setup_command: string;
  test_command?: string;
  lint_command?: string;
  typecheck_command?: string;
  build_command?: string;
  instructions_file?: string;
  manifest_fingerprint: string;
  baseline_commit?: string;
  setup_result: "pending" | "skipped" | "passed" | "failed";
  baseline_result: "pending" | "clean" | "changed" | "dirty";
  cache_path: string;
  prepared_at?: string;
}

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
  preparation_stage: PreparationStage;
  preparation_detail?: string;
  preparation_failed_stage?: PreparationStage;
  selected_project_root?: string;
  configuration_candidates: ProjectCandidate[];
  project_profile?: ProjectProfile;
  setup_command: string;
  path: string;
  sandbox_name: string;
  status: WorkspaceStatus;
  error?: string;
  pull_request_number?: number;
  pull_request_url?: string;
  archived_at?: string;
  created_at: string;
  updated_at: string;
}

export interface WorkspaceSession {
  id: string;
  workspace_id: string;
  title: string;
  status: SessionStatus;
  error?: string;
  selected_agent_id: string;
  created_at: string;
  updated_at: string;
}

export interface AgentDefinition {
  id: string;
  name: string;
  emoji: string;
  description: string;
  provider_id: "fireworks";
  model: string;
  max_steps: number;
  shell_enabled: boolean;
  instructions: string;
  skill_ids: string[];
  revision: number;
  built_in: boolean;
  default: boolean;
  archived_at?: string;
  created_at: string;
  updated_at: string;
}

export interface AgentInput {
  name: string;
  emoji: string;
  description: string;
  provider_id: "fireworks";
  model: string;
  max_steps: number;
  shell_enabled: boolean;
  instructions: string;
  skill_ids: string[];
}

export interface SkillDefinition {
  id: string;
  name: string;
  description: string;
  markdown: string;
  revision: number;
  attached_agents: number;
  archived_at?: string;
  created_at: string;
  updated_at: string;
}

export interface SkillInput {
  name: string;
  description: string;
  markdown: string;
}

export interface SkillReference {
  id: string;
  name: string;
  revision: number;
}

export interface AgentAttribution {
  id: string;
  name: string;
  emoji: string;
  revision: number;
  provider_id: string;
  model: string;
  skills?: SkillReference[];
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
  agent?: AgentAttribution;
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
  branch_mode: BranchMode;
  authority: WorkspaceAuthority;
  setup_command: string;
  environment: EnvironmentInput[];
}

export interface CreateNewProjectInput {
  name: string;
  description: string;
  private: boolean;
  authority: WorkspaceAuthority;
  branch: string;
  setup_command: string;
  environment: EnvironmentInput[];
}

export interface AuthorityChangeInput {
  authority: WorkspaceAuthority;
  branch: string;
  create_branch: boolean;
}

export interface PublishInput {
  commit_message: string;
  title: string;
  body: string;
}
