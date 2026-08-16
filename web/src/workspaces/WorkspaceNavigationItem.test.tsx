import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Workspace } from "../api/contracts";
import { WorkspaceNavigationItem } from "./WorkspaceNavigationItem";

const workspace: Workspace = {
  id: "workspace-1",
  repository: "owner/project",
  clone_url: "https://github.com/owner/project.git",
  base_branch: "main",
  branch: "main",
  create_branch: false,
  authority: "explore",
  preparation_stage: "failed",
  configuration_candidates: [],
  setup_command: "",
  path: "/workspace",
  sandbox_name: "ayati-workspace-1",
  status: "initialization_failed",
  created_at: "2026-08-16T00:00:00Z",
  updated_at: "2026-08-16T00:00:00Z",
};

describe("WorkspaceNavigationItem recovery actions", () => {
  it("retries failed preparation instead of bypassing it", async () => {
    const onAction = vi.fn();
    renderItem(workspace, onAction);
    await userEvent.setup().click(screen.getByRole("button", { name: "Retry preparation" }));
    expect(onAction).toHaveBeenCalledWith("initialize");
    expect(screen.queryByRole("button", { name: "Resume environment" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Stop environment" })).toBeNull();
  });

  it("resumes a stopped prepared sandbox without initialization", async () => {
    const onAction = vi.fn();
    renderItem({ ...workspace, status: "stopped", preparation_stage: "ready" }, onAction);
    await userEvent.setup().click(screen.getByRole("button", { name: "Resume environment" }));
    expect(onAction).toHaveBeenCalledWith("resume");
    expect(screen.queryByRole("button", { name: "Retry preparation" })).toBeNull();
  });
});

function renderItem(value: Workspace, onAction: (action: "initialize" | "resume" | "stop") => void) {
  render(
    <WorkspaceNavigationItem
      workspace={value}
      sessions={[]}
      expanded
      activeWorkspaceID=""
      activeSessionID=""
      onToggle={vi.fn()}
      onOpenSession={vi.fn()}
      onCreateSession={vi.fn()}
      onRenameSession={vi.fn()}
      onDeleteSession={vi.fn()}
      onAction={onAction}
      onDelete={vi.fn()}
    />,
  );
}
