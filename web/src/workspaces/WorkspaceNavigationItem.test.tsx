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
  branch: "perpetual/navigation",
  create_branch: true,
  preparation_stage: "ready",
  configuration_candidates: [],
  setup_command: "",
  path: "/workspace",
  status: "ready",
  created_at: "2026-08-16T00:00:00Z",
  updated_at: "2026-08-16T00:00:00Z",
};

describe("WorkspaceNavigationItem", () => {
  it("continues a ready workspace while keeping details directly available", async () => {
    const onOpenConversation = vi.fn();
    const onOpenOverview = vi.fn();
    const user = userEvent.setup();
    render(<WorkspaceNavigationItem workspace={workspace} active onOpenConversation={onOpenConversation} onOpenOverview={onOpenOverview} />);

    const button = screen.getByRole("button", { name: /continue.*project/i });
    expect(button.getAttribute("aria-current")).toBe("page");
    expect(screen.getByText("perpetual/navigation")).toBeTruthy();
    expect(screen.queryByText(/session/i)).toBeNull();
    await user.click(button);
    expect(onOpenConversation).toHaveBeenCalledOnce();
    await user.click(screen.getByRole("button", { name: "View project workspace details" }));
    expect(onOpenOverview).toHaveBeenCalledOnce();
  });

  it("opens setup instead of conversation when the workspace is not ready", async () => {
    const onOpenConversation = vi.fn();
    const onOpenOverview = vi.fn();
    const user = userEvent.setup();
    render(<WorkspaceNavigationItem workspace={{ ...workspace, status: "initializing" }} active={false} onOpenConversation={onOpenConversation} onOpenOverview={onOpenOverview} />);

    await user.click(screen.getByRole("button", { name: /open.*project/i }));
    expect(onOpenOverview).toHaveBeenCalledOnce();
    expect(onOpenConversation).not.toHaveBeenCalled();
  });
});
