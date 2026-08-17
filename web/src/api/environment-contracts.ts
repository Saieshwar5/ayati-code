export type ComputeEnvironmentState =
  | "provisioning"
  | "available"
  | "occupied"
  | "releasing"
  | "failed"
  | "deleting";

export interface EnvironmentLease {
  id: string;
  environment_id: string;
  workspace_id: string;
  generation: number;
  state: "acquiring" | "active" | "releasing" | "released" | "failed";
  runtime_id?: string;
  error?: string;
  acquired_at: string;
  activated_at?: string;
  released_at?: string;
}

export interface ComputeEnvironment {
  id: string;
  name: string;
  driver: "docker";
  image_ref: string;
  image_digest?: string;
  cpu_millis: number;
  memory_mb: number;
  pid_limit: number;
  network_policy: "disabled" | "outbound";
  provisioning_state: "provisioning" | "ready" | "failed" | "deleting";
  state: ComputeEnvironmentState;
  generation: number;
  error?: string;
  quarantined: boolean;
  active_lease?: EnvironmentLease;
  created_at: string;
  updated_at: string;
}

export interface CreateComputeEnvironmentInput {
  name: string;
  image_ref: string;
  cpu_millis: number;
  memory_mb: number;
  pid_limit: number;
  network_policy: "disabled" | "outbound";
}
