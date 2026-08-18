import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
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
    expect(screen.getByRole("heading", { name: "Workspace profile" })).toBeTruthy();
    expect(screen.getByText("apps/web")).toBeTruthy();
    expect(screen.getByText("Node 22")).toBeTruthy();
    expect(screen.getByText("1234567890ab")).toBeTruthy();
    expect(view.container.textContent).not.toContain("/private/controller");
  });

});
