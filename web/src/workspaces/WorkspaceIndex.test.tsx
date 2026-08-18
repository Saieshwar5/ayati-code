import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
  created_at: "2026-08-17T00:00:00Z",
  updated_at: "2026-08-18T00:00:00Z",
};

describe("WorkspaceIndex", () => {
  it("presents workspaces as compact rows that open from their identity", async () => {
    const onOpen = vi.fn();
    const user = userEvent.setup();
    renderIndex({ workspaces: [workspace], onOpen });

    await user.click(screen.getByRole("button", { name: /perpetual.*feature\/compact-home.*open workspace/i }));

    expect(onOpen).toHaveBeenCalledWith("workspace-1");
    const table = screen.getByLabelText("Active workspaces");
    expect(screen.getByText("Develop")).toBeTruthy();
    expect(within(table).getByText("Ready")).toBeTruthy();
  });

  it("searches, filters, and sorts the active list", async () => {
    const user = userEvent.setup();
    const stopped = { ...workspace, id: "workspace-2", repository: "owner/api", branch: "main", status: "stopped" as const, updated_at: "2026-08-19T00:00:00Z" };
    const preparing = { ...workspace, id: "workspace-3", repository: "owner/web", branch: "release", status: "initializing" as const, updated_at: "2026-08-16T00:00:00Z" };
    renderIndex({ workspaces: [workspace, stopped, preparing] });

    let rows = screen.getAllByRole("button", { name: /open workspace/i });
    expect(rows[0].textContent).toContain("api");
    await user.selectOptions(screen.getByLabelText("Sort workspaces"), "name");
    rows = screen.getAllByRole("button", { name: /open workspace/i });
    expect(rows.map((row) => row.textContent)).toEqual(expect.arrayContaining([expect.stringContaining("api"), expect.stringContaining("perpetual"), expect.stringContaining("web")]));

    await user.selectOptions(screen.getByLabelText("Filter workspaces"), "stopped");
    expect(screen.getAllByRole("button", { name: /open workspace/i })).toHaveLength(1);
    expect(within(screen.getByLabelText("Active workspaces")).getByText("Stopped")).toBeTruthy();
    await user.selectOptions(screen.getByLabelText("Filter workspaces"), "all");
    await user.type(screen.getByRole("searchbox", { name: "Search workspaces" }), "release");
    expect(screen.getAllByRole("button", { name: /open workspace/i })).toHaveLength(1);
    expect(within(screen.getByLabelText("Active workspaces")).getByText("Preparing")).toBeTruthy();
  });

  it("runs lifecycle actions without opening the workspace", async () => {
    const onOpen = vi.fn();
    const onStop = vi.fn().mockResolvedValue(undefined);
    const onArchive = vi.fn().mockResolvedValue(true);
    const user = userEvent.setup();
    renderIndex({ workspaces: [workspace], onOpen, onStop, onArchive });

    await user.click(screen.getByRole("button", { name: "Stop" }));
    expect(onStop).toHaveBeenCalledWith(workspace);
    expect(onOpen).not.toHaveBeenCalled();
    await user.click(screen.getByLabelText("More actions for perpetual"));
    await user.click(screen.getByRole("button", { name: "Archive workspace…" }));
    expect(onArchive).toHaveBeenCalledWith(workspace);
    expect(onOpen).not.toHaveBeenCalled();
  });

  it("manages archived workspaces in the same searchable surface", async () => {
    const archived = { ...workspace, archived_at: "2026-08-19T00:00:00Z", status: "stopped" as const };
    const onRestore = vi.fn().mockResolvedValue(undefined);
    const onDelete = vi.fn().mockResolvedValue(true);
    const onViewChange = vi.fn();
    const user = userEvent.setup();
    renderIndex({ workspaces: [], archivedWorkspaces: [archived], view: "archived", onRestore, onDelete, onViewChange });

    expect(screen.getByLabelText("Archived workspaces")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /open workspace/i })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Restore" }));
    expect(onRestore).toHaveBeenCalledWith(archived);
    await user.click(screen.getByLabelText("More actions for perpetual"));
    await user.click(screen.getByRole("button", { name: "Delete workspace…" }));
    expect(onDelete).toHaveBeenCalledWith(archived);
    await user.click(screen.getByRole("radio", { name: /Active0/ }));
    expect(onViewChange).toHaveBeenCalledWith("active");
  });

  it("keeps a failed local deletion visible and retryable", async () => {
    const failed = { ...workspace, status: "deletion_failed" as const, error: "read-only cache" };
    const onDelete = vi.fn().mockResolvedValue(true);
    const user = userEvent.setup();
    renderIndex({ workspaces: [failed], onDelete });

    expect(within(screen.getByLabelText("Active workspaces")).getByText("Deletion failed")).toBeTruthy();
    await user.click(screen.getByLabelText("More actions for perpetual"));
    await user.click(screen.getByRole("button", { name: "Retry deletion…" }));
    expect(onDelete).toHaveBeenCalledWith(failed);
  });

  it("keeps empty and no-result states specific", async () => {
    const user = userEvent.setup();
    renderIndex({ workspaces: [] });
    expect(screen.getByRole("heading", { name: "Create your first workspace" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Create workspace" })).toBeTruthy();

    renderIndex({ workspaces: [workspace] });
    const managers = screen.getAllByRole("searchbox", { name: "Search workspaces" });
    await user.type(managers.at(-1)!, "missing");
    expect(screen.getByRole("heading", { name: "No matching workspaces" })).toBeTruthy();
  });
});

function renderIndex(overrides: Partial<React.ComponentProps<typeof WorkspaceIndex>> = {}) {
  const props: React.ComponentProps<typeof WorkspaceIndex> = {
    workspaces: [],
    archivedWorkspaces: [],
    view: "active",
    onViewChange: vi.fn(),
    onCreate: vi.fn(),
    onOpen: vi.fn(),
    onStop: vi.fn().mockResolvedValue(undefined),
    onResume: vi.fn().mockResolvedValue(undefined),
    onArchive: vi.fn().mockResolvedValue(true),
    onRestore: vi.fn().mockResolvedValue(undefined),
    onDelete: vi.fn().mockResolvedValue(true),
    ...overrides,
  };
  return render(<WorkspaceIndex {...props} />);
}
