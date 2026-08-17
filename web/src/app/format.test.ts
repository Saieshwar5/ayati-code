import { describe, expect, it } from "vitest";
import type { WorkspaceSession } from "../api/contracts";
import { repositoryName, sessionMeta, statusLabel } from "./format";

describe("format helpers", () => {
  it("formats repository and status labels", () => {
    expect(repositoryName("owner/project")).toBe("project");
    expect(statusLabel("initialization_failed")).toBe("initialization failed");
  });

  it("describes recent idle sessions", () => {
    const now = Date.parse("2026-08-16T12:00:00Z");
    const session: WorkspaceSession = {
      id: "session-1",
      workspace_id: "workspace-1",
      title: "Session",
      status: "idle",
      selected_agent_id: "builtin-ayati",
      created_at: "2026-08-16T11:55:00Z",
      updated_at: "2026-08-16T11:55:00Z",
    };
    expect(sessionMeta(session, now)).toBe("5m ago");
  });
});
