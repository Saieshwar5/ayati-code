import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Workspace, WorkspaceSession } from "../api/contracts";
import { EnvironmentPanel } from "./EnvironmentPanel";

const workspace: Workspace = {
  id: "workspace-1",
  repository: "owner/project",
  clone_url: "https://github.com/owner/project.git",
  base_branch: "main",
  branch: "perpetual/change",
  create_branch: false,
  preparation_stage: "ready",
  configuration_candidates: [],
  setup_command: "",
  path: "/workspace",
  status: "ready",
  created_at: "2026-08-16T00:00:00Z",
  updated_at: "2026-08-16T00:00:00Z",
};

const session: WorkspaceSession = {
  id: "session-1",
  workspace_id: workspace.id,
  title: "Session",
  status: "idle",
  selected_agent_id: "builtin-ayati",
  created_at: "2026-08-16T00:00:00Z",
  updated_at: "2026-08-16T00:00:00Z",
};

afterEach(() => vi.restoreAllMocks());

describe("EnvironmentPanel", () => {
  it("lists write-only metadata and replaces a variable", async () => {
    let update: RequestInit | undefined;
    vi.spyOn(globalThis, "fetch").mockImplementation(async (_, init) => {
      if (init?.method === "POST") {
        update = init;
        return json({ name: "NPM_TOKEN", configured: true, expose_during_setup: true });
      }
      return json([
        {
          name: "NPM_TOKEN",
          configured: true,
          expose_during_setup: true,
          updated_at: "2026-08-16T00:00:00Z",
        },
      ]);
    });

    const user = userEvent.setup();
    render(<EnvironmentPanel workspace={workspace} sessions={[session]} />);
    expect(await screen.findByText("NPM_TOKEN")).toBeTruthy();
    expect(screen.queryByText("private-token")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Replace" }));
    await user.type(screen.getByLabelText("Value"), "replacement-token");
    await user.click(screen.getByRole("button", { name: "Replace value" }));

    await waitFor(() => expect(update).toBeTruthy());
    expect(new Headers(update?.headers).get("X-Perpetual-Request")).toBe("1");
    expect(JSON.parse(String(update?.body))).toEqual({
      name: "NPM_TOKEN",
      value: "replacement-token",
      expose_during_setup: true,
    });
  });

  it("locks mutations while another session is working", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(json([]));
    render(
      <EnvironmentPanel
        workspace={workspace}
        sessions={[session, { ...session, id: "session-2", status: "working" }]}
      />,
    );
    expect(await screen.findByText(/while an agent is working/)).toBeTruthy();
    expect((screen.getByRole("button", { name: "Add variable" }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });
});

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
