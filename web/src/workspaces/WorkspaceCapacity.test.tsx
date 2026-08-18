import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { Workspace } from "../api/contracts";
import type { ComputeEnvironment } from "../api/environment-contracts";
import { WorkspaceCapacity } from "./WorkspaceCapacity";

const workspace: Workspace = {
  id: "workspace-1",
  repository: "owner/project",
  clone_url: "https://github.com/owner/project.git",
  base_branch: "main",
  branch: "perpetual/capacity",
  create_branch: true,
  preparation_stage: "ready",
  configuration_candidates: [],
  setup_command: "",
  path: "/workspace",
  status: "ready",
  created_at: "2026-08-17T00:00:00Z",
  updated_at: "2026-08-17T00:00:00Z",
};

const environment: ComputeEnvironment = {
  id: "environment-1",
  name: "Local Docker",
  driver: "docker",
  image_ref: "perpetual-sandbox:dev",
  cpu_millis: 2000,
  memory_mb: 4096,
  pid_limit: 256,
  network_policy: "outbound",
  provisioning_state: "ready",
  state: "occupied",
  generation: 1,
  quarantined: false,
  active_lease: {
    id: "lease-1",
    environment_id: "environment-1",
    workspace_id: workspace.id,
    generation: 1,
    state: "active",
    acquired_at: "2026-08-17T00:00:00Z",
  },
  created_at: "2026-08-17T00:00:00Z",
  updated_at: "2026-08-17T00:00:00Z",
};

afterEach(() => vi.restoreAllMocks());

it("shows the environment assigned to the workspace", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(json([environment]));
  const onManage = vi.fn();
  const user = userEvent.setup();

  render(<WorkspaceCapacity workspace={workspace} onManage={onManage} />);

  const capacity = await screen.findByRole("button", {
    name: /Environment: Local Docker\. 2 CPU · 4 GB/i,
  });
  await user.click(capacity);
  expect(onManage).toHaveBeenCalledOnce();
});

it("reports when every environment is occupied elsewhere", async () => {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(json([{
    ...environment,
    active_lease: { ...environment.active_lease!, workspace_id: "workspace-2" },
  }]));

  render(<WorkspaceCapacity workspace={{ ...workspace, status: "stopped" }} onManage={() => {}} />);

  expect(await screen.findByRole("button", { name: /Environment: No capacity/i })).toBeTruthy();
  expect(screen.getByText("Add or release an environment")).toBeTruthy();
});

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}
