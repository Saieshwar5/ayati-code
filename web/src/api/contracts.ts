export type WorkspaceStatus =
  | "creating"
  | "initializing"
  | "waiting_environment"
  | "initialization_failed"
  | "needs_configuration"
  | "ready"
  | "stopped"
  | "deleting"
  | "deletion_failed";

export type SessionStatus = "idle" | "working" | "review" | "failed" | "canceled";
export type BranchMode = "new" | "existing" | "direct";
export type PreparationStage =
  | "pending"
  | "cloning"
  | "analyzing"
  | "starting_environment"
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

export interface Toolchain {
  name: string;
  version?: string;
  source?: string;
}

export interface DevService {
  name: string;
  version?: string;
  source?: string;
}

export interface EnvironmentSpec {
  project_root: string;
  toolchains: Toolchain[];
  package_managers: string[];
  lockfiles: string[];
  setup_commands: string[];
  verify_commands: string[];
  build_commands: string[];
  test_commands: string[];
  services: DevService[];
  source_files: string[];
  fingerprint: string;
  instructions_file?: string;
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
  environment_spec?: EnvironmentSpec;
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
  user_id?: string;
  repository: string;
  clone_url: string;
  base_branch: string;
  branch: string;
  create_branch: boolean;
  environment_version_id?: string;
  preparation_stage: PreparationStage;
  preparation_detail?: string;
  preparation_failed_stage?: PreparationStage;
  selected_project_root?: string;
  configuration_candidates: ProjectCandidate[];
  project_profile?: ProjectProfile;
  setup_command: string;
  path: string;
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
  active_run_id?: string;
  created_at: string;
  updated_at: string;
}

export interface AgentRun {
  id: string;
  workspace_id: string;
  session_id: string;
  status: "accepted" | "running" | "completed" | "failed" | "canceled" | "interrupted";
  error?: string;
  created_at: string;
  started_at?: string;
  finished_at?: string;
  updated_at: string;
}

export interface ToolCall {
  id: string;
  type: string;
  function: { name: string; arguments: string };
}

export interface Message {
  id?: number;
  role: "user" | "assistant" | "tool" | string;
  content?: string;
  tool_calls?: ToolCall[];
  tool_call_id?: string;
  created_at?: string;
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
  setup_command: string;
  environment: EnvironmentInput[];
}

export interface CreateNewProjectInput {
  name: string;
  description: string;
  private: boolean;
  branch: string;
  setup_command: string;
  environment: EnvironmentInput[];
}

export interface PublishInput {
  commit_message: string;
  title: string;
  body: string;
}
