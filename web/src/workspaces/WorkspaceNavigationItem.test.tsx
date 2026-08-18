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
  it("opens the workspace without exposing session or lifecycle actions", async () => {
    const onOpen = vi.fn();
    render(<WorkspaceNavigationItem workspace={workspace} active onOpen={onOpen} />);

    const button = screen.getByRole("button", { name: /project/i });
    expect(button.getAttribute("aria-current")).toBe("page");
    expect(screen.getByText("perpetual/navigation")).toBeTruthy();
    expect(screen.queryByText(/session/i)).toBeNull();
    await userEvent.setup().click(button);
    expect(onOpen).toHaveBeenCalledOnce();
  });
});
