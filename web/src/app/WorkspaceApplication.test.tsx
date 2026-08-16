import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Workspace, WorkspaceSession } from "../api/contracts";
import { WorkspaceApplication } from "./WorkspaceApplication";

const workspace: Workspace = {
  id: "workspace-1",
  repository: "owner/project",
  clone_url: "https://github.com/owner/project.git",
  base_branch: "main",
  branch: "ayati/react-ui",
  create_branch: false,
  setup_command: "go mod download",
  path: "/workspace",
  sandbox_name: "ayati-workspace-1",
  status: "creating",
  created_at: "2026-08-16T00:00:00Z",
  updated_at: "2026-08-16T00:00:00Z",
};

const session: WorkspaceSession = {
  id: "session-1",
  workspace_id: workspace.id,
  title: "Original session",
  status: "idle",
  created_at: "2026-08-16T00:00:00Z",
  updated_at: "2026-08-16T00:00:00Z",
};

afterEach(() => vi.restoreAllMocks());

describe("WorkspaceApplication", () => {
  it("creates a workspace from an installed repository", async () => {
    let created = false;
    let createRequest: RequestInit | undefined;
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === "/api/repositories") {
        return json([{ id: 1, full_name: "owner/project", default_branch: "main" }]);
      }
      if (path === "/api/workspaces" && init?.method === "POST") {
        created = true;
        createRequest = init;
        return json(workspace, 202);
      }
      if (path === "/api/workspaces") return json(created ? [workspace] : []);
      if (path === "/api/repositories/owner/project/branches") {
        return json([{ name: "main", commit: { sha: "abc123" } }]);
      }
      if (path === `/api/workspaces/${workspace.id}/sessions`) return json([session]);
      if (path === `/api/workspaces/${workspace.id}/sessions/${session.id}/messages`) return json([]);
      throw new Error(`Unexpected request: ${init?.method || "GET"} ${path}`);
    });

    const user = userEvent.setup();
    render(
      <WorkspaceApplication
        user={{ id: 1, login: "octocat", avatar_url: "https://example.test/avatar.png" }}
      />,
    );
    await screen.findByRole("heading", { name: "Select a workspace" });
    await user.click(screen.getByRole("button", { name: /new workspace/i }));
    await user.selectOptions(screen.getByLabelText("Repository"), "owner/project");
    await waitFor(() =>
      expect((screen.getByLabelText("Base branch") as HTMLSelectElement).value).toBe("main"),
    );
    await user.type(screen.getByLabelText("New working branch"), "ayati/react-ui");
    await user.click(screen.getByRole("button", { name: "Add variable" }));
    await user.type(screen.getByLabelText("Name"), "NPM_TOKEN");
    await user.type(screen.getByLabelText("Value"), "private-token");
    await user.click(screen.getByLabelText("During setup"));
    await user.click(screen.getByRole("button", { name: "Create and initialize" }));

    expect(await screen.findByRole("heading", { name: "Original session" })).toBeTruthy();
    expect(new Headers(createRequest?.headers).get("X-Ayati-Request")).toBe("1");
    expect(JSON.parse(String(createRequest?.body))).toMatchObject({
      repository: "owner/project",
      base_branch: "main",
      branch: "ayati/react-ui",
      create_branch: true,
      environment: [
        { name: "NPM_TOKEN", value: "private-token", expose_during_setup: true },
      ],
    });
  });
});

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}
