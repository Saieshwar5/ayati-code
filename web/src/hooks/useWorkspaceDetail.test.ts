import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Message, Workspace, WorkspaceSession } from "../api/contracts";
import { reconcileMessages, useWorkspaceDetail } from "./useWorkspaceDetail";

const apiMock = vi.hoisted(() => ({
  messages: vi.fn(),
  changes: vi.fn(),
  sessionByID: vi.fn(),
  sendMessage: vi.fn(),
  cancelRun: vi.fn(),
  publish: vi.fn(),
}));

vi.mock("../api/client", () => ({ api: apiMock }));

const workspace: Workspace = {
  id: "workspace-1", repository: "owner/project", clone_url: "https://github.com/owner/project.git",
  base_branch: "main", branch: "perpetual/activity", create_branch: true,
  preparation_stage: "ready", configuration_candidates: [], setup_command: "", path: "/workspace",
  status: "ready", created_at: "2026-08-18T00:00:00Z", updated_at: "2026-08-18T00:00:00Z",
};

const session: WorkspaceSession = {
  id: "session-1", workspace_id: workspace.id, title: "Activity", status: "working",
  selected_agent_id: "builtin-ayati", active_run_id: "run-1",
  created_at: "2026-08-18T00:00:00Z", updated_at: "2026-08-18T00:00:00Z",
};

beforeEach(() => {
  Object.values(apiMock).forEach((mock) => mock.mockReset());
  apiMock.changes.mockResolvedValue({ status: "", diff: "" });
});

describe("reconcileMessages", () => {
  it("keeps existing message objects stable while appending server updates", () => {
    const current: Message[] = [{ id: 1, role: "user", content: "Inspect the project" }];
    const incoming: Message[] = [
      { id: 1, role: "user", content: "Inspect the project" },
      { id: 2, role: "assistant", content: "Working on it" },
    ];

    const reconciled = reconcileMessages(current, incoming);

    expect(reconciled).toHaveLength(2);
    expect(reconciled[0]).toBe(current[0]);
    expect(reconciled[1]).toBe(incoming[1]);
    expect(reconcileMessages(reconciled, [...incoming])).toBe(reconciled);
  });

  it("replaces the list when durable message identity diverges", () => {
    const current: Message[] = [{ id: 1, role: "user", content: "Old" }];
    const incoming: Message[] = [{ id: 2, role: "user", content: "Fresh context" }];

    expect(reconcileMessages(current, incoming)).toBe(incoming);
  });

  it("keeps messages visible when a server event replaces the session object", async () => {
    const message: Message = { id: 1, role: "user", content: "Inspect the project" };
    apiMock.messages.mockResolvedValue([message]);
    const onSessionUpdate = vi.fn();
    const onWorkspaceUpdate = vi.fn();
    const { result, rerender } = renderHook(
      ({ currentSession }) => useWorkspaceDetail({
        workspace, session: currentSession, onSessionUpdate, onWorkspaceUpdate,
      }),
      { initialProps: { currentSession: session } },
    );

    await waitFor(() => expect(result.current.messages).toHaveLength(1));
    const stableMessage = result.current.messages[0];
    rerender({ currentSession: { ...session, updated_at: "2026-08-18T00:00:01Z" } });

    expect(result.current.messages).toHaveLength(1);
    expect(result.current.messages[0]).toBe(stableMessage);
    expect(apiMock.messages).toHaveBeenCalledTimes(1);
  });
});
