import { useEffect, useState } from "react";
import type { Workspace } from "../api/contracts";
import { api } from "../api/client";
import type { ComputeEnvironment } from "../api/environment-contracts";

interface WorkspaceCapacityProps {
  workspace: Workspace;
  onManage: () => void;
}

export function WorkspaceCapacity({ workspace, onManage }: WorkspaceCapacityProps) {
  const [environments, setEnvironments] = useState<ComputeEnvironment[]>([]);
  const [state, setState] = useState<"loading" | "ready" | "error">("loading");

  useEffect(() => {
    let current = true;
    setState("loading");
    api.environments().then(
      (values) => {
        if (!current) return;
        setEnvironments(values);
        setState("ready");
      },
      () => {
        if (current) setState("error");
      },
    );
    return () => { current = false; };
  }, [workspace.id, workspace.status, workspace.updated_at]);

  const assigned = environments.find(
    (value) => value.active_lease?.workspace_id === workspace.id,
  );
  const available = environments.filter((value) => value.state === "available").length;
  const presentation = capacityPresentation(state, assigned, available);

  return (
    <button
      className={`workspace-capacity ${presentation.tone}`}
      type="button"
      aria-label={`Environment: ${presentation.label}. ${presentation.detail}. Manage environments`}
      onClick={onManage}
    >
      <span className="workspace-capacity-dot" aria-hidden="true" />
      <span>
        <small>Environment</small>
        <strong>{presentation.label}</strong>
        <em>{presentation.detail}</em>
      </span>
    </button>
  );
}

function capacityPresentation(
  state: "loading" | "ready" | "error",
  assigned: ComputeEnvironment | undefined,
  available: number,
) {
  if (state === "loading") {
    return { label: "Checking capacity", detail: "Automatic allocation", tone: "loading" };
  }
  if (state === "error") {
    return { label: "Capacity unavailable", detail: "Open environments", tone: "failed" };
  }
  if (assigned) {
    return {
      label: assigned.name,
      detail: `${formatCPU(assigned.cpu_millis)} · ${formatMemory(assigned.memory_mb)}`,
      tone: assigned.state === "releasing" ? "releasing" : "assigned",
    };
  }
  if (available > 0) {
    return {
      label: `${available} available`,
      detail: "Assigned automatically on start",
      tone: "available",
    };
  }
  return { label: "No capacity", detail: "Add or release an environment", tone: "failed" };
}

function formatCPU(millis: number): string {
  return `${Number((millis / 1000).toFixed(1))} CPU`;
}

function formatMemory(megabytes: number): string {
  return megabytes >= 1024
    ? `${Number((megabytes / 1024).toFixed(1))} GB`
    : `${megabytes} MB`;
}
