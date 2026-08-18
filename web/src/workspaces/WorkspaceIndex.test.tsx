import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Workspace } from "../api/contracts";
import { WorkspaceIndex } from "./WorkspaceIndex";

const workspace: Workspace = {
  id: "workspace-1",
  repository: "owner/perpetual",
  clone_url: "https://github.com/owner/perpetual.git",
  base_branch: "main",
  branch: "feature/compact-home",
  create_branch: true,
  authority: "develop",
  effective_mount_mode: "rw",
  preparation_stage: "ready",
  preparation_detail: "",
  configuration_candidates: [],
  setup_command: "npm ci",
  path: "/workspace",
  status: "ready",
  created_at: "2026-08-18T00:00:00Z",
  updated_at: "2026-08-18T00:00:00Z",
};

describe("WorkspaceIndex", () => {
  it("presents workspaces as compact accessible rows", () => {
    const onOpen = vi.fn();
    render(<WorkspaceIndex workspaces={[workspace]} onCreate={() => {}} onOpen={onOpen} />);

    const row = screen.getByRole("button", { name: /perpetual.*feature\/compact-home.*ready/i });
    row.click();

    expect(onOpen).toHaveBeenCalledWith("workspace-1");
    expect(screen.getByLabelText("Workspaces")).toBeTruthy();
  });

  it("keeps the empty state focused on its primary action", () => {
    render(<WorkspaceIndex workspaces={[]} onCreate={() => {}} onOpen={() => {}} />);

    expect(screen.getByRole("heading", { name: "Create your first workspace" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Create workspace" })).toBeTruthy();
  });
});
