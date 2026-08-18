import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Workspace } from "../api/contracts";
import { WorkspaceReadiness } from "./WorkspaceReadiness";

const workspace: Workspace = {
  id: "workspace-1",
  repository: "owner/project",
  clone_url: "https://github.com/owner/project.git",
  base_branch: "main",
  branch: "main",
  create_branch: false,
  authority: "explore",
  preparation_stage: "analyzing",
  preparation_detail: "Inspecting project metadata",
  configuration_candidates: [],
  setup_command: "",
  path: "/private/managed/path",
  status: "initializing",
  created_at: "2026-08-16T00:00:00Z",
  updated_at: "2026-08-16T00:00:00Z",
};

describe("WorkspaceReadiness", () => {
  it("shows completed, current, and pending preparation steps", () => {
    render(<WorkspaceReadiness workspace={workspace} onConfigure={vi.fn()} onRetry={vi.fn()} onResume={vi.fn()} onDelete={vi.fn()} />);
    expect(screen.getByText("Repository").closest("li")?.className).toBe("done");
    expect(screen.getAllByText("Project").find((element) => element.closest("li"))?.closest("li")?.className).toBe("current");
    expect(screen.getByText("Dependencies").closest("li")?.className).toBe("pending");
    expect(screen.getByText("Inspecting project metadata")).toBeTruthy();
  });

  it("submits the selected project root", async () => {
    const configure = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(
      <WorkspaceReadiness
        workspace={{
          ...workspace,
          status: "needs_configuration",
          preparation_stage: "needs_configuration",
          configuration_candidates: [
            { project_root: "apps/api", languages: ["Go"], package_managers: ["Go modules"] },
            { project_root: "apps/web", languages: ["Node.js"], package_managers: ["npm"] },
          ],
        }}
        onConfigure={configure}
        onRetry={vi.fn()}
        onResume={vi.fn()}
        onDelete={vi.fn()}
      />,
    );
    await user.click(screen.getByRole("radio", { name: /apps\/web/i }));
    await user.click(screen.getByRole("button", { name: "Continue preparation" }));
    expect(configure).toHaveBeenCalledWith("apps/web");
  });

  it("shows the failed stage and retries without hiding the error", async () => {
    const retry = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(
      <WorkspaceReadiness
        workspace={{
          ...workspace,
          status: "initialization_failed",
          preparation_stage: "failed",
          preparation_failed_stage: "verifying",
          error: "setup modified package-lock.json",
        }}
        onConfigure={vi.fn()}
        onRetry={retry}
        onResume={vi.fn()}
        onDelete={vi.fn()}
      />,
    );
    expect(screen.getByRole("heading", { name: "Verify needs attention" })).toBeTruthy();
    expect(screen.getByText("Verify could not finish")).toBeTruthy();
    expect(screen.getByText("setup modified package-lock.json")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Retry preparation" }));
    expect(retry).toHaveBeenCalledOnce();
  });

  it("resumes a stopped sandbox without retrying preparation", async () => {
    const retry = vi.fn().mockResolvedValue(undefined);
    const resume = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(
      <WorkspaceReadiness
        workspace={{ ...workspace, status: "stopped", preparation_stage: "ready" }}
        onConfigure={vi.fn()}
        onRetry={retry}
        onResume={resume}
        onDelete={vi.fn()}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Resume environment" }));
    expect(resume).toHaveBeenCalledOnce();
    expect(retry).not.toHaveBeenCalled();
  });
});
