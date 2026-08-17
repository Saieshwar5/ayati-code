import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { ComputeEnvironment } from "../api/environment-contracts";
import type { Workspace } from "../api/contracts";
import { EnvironmentsPage } from "./EnvironmentsPage";

const baseEnvironment: ComputeEnvironment = {
  id: "environment-1",
  name: "Local Docker",
  driver: "docker",
  image_ref: "ayati-sandbox:dev",
  image_digest: `sha256:${"a".repeat(64)}`,
  cpu_millis: 2000,
  memory_mb: 4096,
  pid_limit: 256,
  network_policy: "outbound",
  provisioning_state: "ready",
  state: "available",
  generation: 2,
  quarantined: false,
  created_at: "2026-08-17T00:00:00Z",
  updated_at: "2026-08-17T00:00:00Z",
};

const workspace: Workspace = {
  id: "workspace-123456789",
  repository: "owner/project",
  clone_url: "https://github.com/owner/project.git",
  base_branch: "main",
  branch: "ayati/capacity",
  create_branch: true,
  authority: "develop",
  preparation_stage: "ready",
  configuration_candidates: [],
  setup_command: "",
  path: "/workspace",
  status: "ready",
  created_at: "2026-08-17T00:00:00Z",
  updated_at: "2026-08-17T00:00:00Z",
};

afterEach(() => vi.restoreAllMocks());

it("shows capacity and keeps occupied environments protected", async () => {
  const occupied: ComputeEnvironment = {
    ...baseEnvironment,
    state: "occupied",
    active_lease: {
      id: "lease-1",
      environment_id: baseEnvironment.id,
      workspace_id: "workspace-123456789",
      generation: 2,
      state: "active",
      acquired_at: "2026-08-17T00:00:00Z",
    },
  };
  vi.spyOn(globalThis, "fetch").mockImplementation(() => json([occupied]));
  const onOpenWorkspace = vi.fn();
  const user = userEvent.setup();
  render(<EnvironmentsPage workspaces={[workspace]} onOpenWorkspace={onOpenWorkspace} />);

  expect(await screen.findByRole("heading", { name: "Local Docker" })).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Open workspace owner/project" }));
  expect(onOpenWorkspace).toHaveBeenCalledWith(workspace.id);
  expect(screen.getByText("project")).toBeTruthy();
  expect(screen.getByText("ayati/capacity")).toBeTruthy();
  expect((screen.getByRole("button", { name: "Delete" }) as HTMLButtonElement).disabled).toBe(true);
  expect(screen.getAllByText("In use")).toHaveLength(2);
});

it("creates and repairs local environment capacity", async () => {
  let values: ComputeEnvironment[] = [];
  let createBody: unknown;
  vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
    const path = String(input);
    if (path === "/api/environments" && init?.method === "POST") {
      createBody = JSON.parse(String(init.body));
      values = [{ ...baseEnvironment, id: "environment-new", name: "Node projects" }];
      return json(values[0], 201);
    }
    if (path === "/api/environments/environment-new/repair" && init?.method === "POST") {
      values = [{ ...values[0], provisioning_state: "ready", state: "available", error: undefined }];
      return json(values[0]);
    }
    if (path === "/api/environments") return json(values);
    throw new Error(`Unexpected request: ${init?.method || "GET"} ${path}`);
  });

  const user = userEvent.setup();
  render(<EnvironmentsPage />);
  await screen.findByRole("heading", { name: "No environment capacity" });
  await user.click(screen.getByRole("button", { name: /New environment/ }));
  await user.type(screen.getByLabelText("Name"), "Node projects");
  await user.click(screen.getByRole("button", { name: "Create environment" }));

  expect(await screen.findByRole("heading", { name: "Node projects" })).toBeTruthy();
  expect(createBody).toMatchObject({
    name: "Node projects",
    image_ref: "ayati-sandbox:dev",
    cpu_millis: 2000,
    memory_mb: 4096,
    pid_limit: 256,
    network_policy: "outbound",
  });

  values = [{
    ...values[0], provisioning_state: "failed", state: "failed", error: "image is missing",
  }];
  window.history.pushState({}, "", "/environments");
  render(<EnvironmentsPage />);
  expect(await screen.findByText("image is missing")).toBeTruthy();
  await user.click(screen.getByRole("button", { name: "Repair" }));
  await waitFor(() => expect(screen.queryByText("image is missing")).toBeNull());
});

function json(body: unknown, status = 200): Promise<Response> {
  return Promise.resolve(new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  }));
}
