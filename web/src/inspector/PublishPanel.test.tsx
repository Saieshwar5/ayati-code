import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Workspace } from "../api/contracts";
import { PublishPanel } from "./PublishPanel";

const workspace: Workspace = {
  id: "workspace-1",
  repository: "owner/project",
  clone_url: "https://github.com/owner/project.git",
  base_branch: "main",
  branch: "main",
  create_branch: false,
  authority: "explore",
  effective_mount_mode: "ro",
  setup_command: "",
  path: "/workspace",
  sandbox_name: "ayati-workspace-1",
  status: "ready",
  created_at: "2026-08-16T00:00:00Z",
  updated_at: "2026-08-16T00:00:00Z",
};

describe("PublishPanel", () => {
  it("does not offer publishing from Explore", () => {
    render(<PublishPanel workspace={workspace} publishing={false} onPublish={vi.fn()} />);
    expect(screen.getByRole("heading", { name: "Publishing protected" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: /pull request/i })).toBeNull();
  });
});
