import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Workspace } from "../api/contracts";
import { WorkspaceProfilePanel } from "./WorkspaceProfilePanel";

describe("WorkspaceProfilePanel", () => {
  it("shows resolved project facts without exposing controller paths", () => {
    const workspace: Workspace = {
      id: "workspace-1",
      repository: "owner/project",
      clone_url: "https://github.com/owner/project.git",
      base_branch: "main",
      branch: "main",
      create_branch: false,
      authority: "explore",
      effective_mount_mode: "ro",
      preparation_stage: "ready",
      preparation_detail: "Workspace ready",
      configuration_candidates: [],
      project_profile: {
        project_root: "apps/web",
        languages: ["Node.js"],
        runtime_versions: ["Node 22"],
        package_managers: ["pnpm"],
        lockfiles: ["pnpm-lock.yaml"],
        setup_command: "cd 'apps/web' && corepack pnpm install --frozen-lockfile",
        test_command: "cd 'apps/web' && corepack pnpm run test",
        manifest_fingerprint: "abc123",
        baseline_commit: "1234567890abcdef",
        setup_result: "passed",
        baseline_result: "clean",
        cache_path: "/private/controller/cache",
      },
      setup_command: "",
      path: "/private/controller/workspace",
      status: "ready",
      created_at: "2026-08-16T00:00:00Z",
      updated_at: "2026-08-16T00:00:00Z",
    };

    const view = render(<WorkspaceProfilePanel workspace={workspace} />);
    expect(screen.getByText("Source protected")).toBeTruthy();
    expect(screen.getByText("apps/web")).toBeTruthy();
    expect(screen.getByText("Node 22")).toBeTruthy();
    expect(screen.getByText("1234567890ab")).toBeTruthy();
    expect(view.container.textContent).not.toContain("/private/controller");
  });

  it("confirms Develop with a local working branch", async () => {
    const change = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    const workspace = exploreWorkspace();
    render(<WorkspaceProfilePanel workspace={workspace} onAuthorityChange={change} />);

    await user.click(screen.getByRole("radio", { name: "Develop" }));
    expect(screen.getByRole("heading", { name: "Enable development?" })).toBeTruthy();
    const branch = screen.getByLabelText("Working branch");
    await user.clear(branch);
    await user.type(branch, "ayati/authority-switch");
    await user.click(screen.getByRole("button", { name: "Enable Develop" }));

    expect(change).toHaveBeenCalledWith({
      authority: "develop",
      branch: "ayati/authority-switch",
      create_branch: true,
    });
  });

  it("disables authority changes while an agent is working", () => {
    render(<WorkspaceProfilePanel workspace={exploreWorkspace()} agentWorking />);
    expect((screen.getByRole("radio", { name: "Develop" }) as HTMLInputElement).disabled).toBe(true);
    expect(screen.getByText("Authority cannot change while the agent is working.")).toBeTruthy();
  });
});

function exploreWorkspace(): Workspace {
  return {
    id: "workspace-2",
    repository: "owner/project",
    clone_url: "https://github.com/owner/project.git",
    base_branch: "main",
    branch: "main",
    create_branch: false,
    authority: "explore",
    effective_mount_mode: "ro",
    preparation_stage: "ready",
    preparation_detail: "Workspace ready",
    configuration_candidates: [],
    setup_command: "",
    path: "/workspace",
    status: "ready",
    created_at: "2026-08-16T00:00:00Z",
    updated_at: "2026-08-16T00:00:00Z",
  };
}
