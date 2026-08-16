import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Workspace, WorkspaceSession } from "../api/contracts";
import { ChatPane } from "./ChatPane";

const workspace: Workspace = {
  id: "workspace-1",
  repository: "owner/project",
  clone_url: "https://github.com/owner/project.git",
  base_branch: "main",
  branch: "ayati/change",
  create_branch: false,
  setup_command: "go mod download",
  path: "/workspace",
  sandbox_name: "ayati-workspace-1",
  status: "ready",
  created_at: "2026-08-16T00:00:00Z",
  updated_at: "2026-08-16T00:00:00Z",
};

const session: WorkspaceSession = {
  id: "session-1",
  workspace_id: workspace.id,
  title: "Improve the UI",
  status: "idle",
  created_at: "2026-08-16T00:00:00Z",
  updated_at: "2026-08-16T00:00:00Z",
};

describe("ChatPane", () => {
  it("renders conversation messages and sends the composer text", async () => {
    const send = vi.fn().mockResolvedValue(true);
    const user = userEvent.setup();
    render(
      <ChatPane
        workspace={workspace}
        session={session}
        workspaceSessions={[session]}
        messages={[
          { role: "user", content: "Please inspect this." },
          {
            role: "assistant",
            tool_calls: [
              { id: "call-1", type: "function", function: { name: "shell", arguments: "{}" } },
            ],
          },
          { role: "assistant", content: "The project is ready." },
        ]}
        error=""
        sending={false}
        onSend={send}
      />,
    );

    expect(screen.getByText("Please inspect this.")).toBeTruthy();
    expect(screen.getByText("The project is ready.")).toBeTruthy();
    expect(screen.queryByText("shell")).toBeNull();
    await user.type(screen.getByRole("textbox"), "Implement the change");
    await user.click(screen.getByRole("button", { name: "Send message" }));
    expect(send).toHaveBeenCalledWith("Implement the change");
    expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("");
  });

  it("blocks this composer while another session is working", () => {
    const working = { ...session, id: "session-2", status: "working" as const };
    render(
      <ChatPane
        workspace={workspace}
        session={session}
        workspaceSessions={[session, working]}
        messages={[]}
        error=""
        sending={false}
        onSend={vi.fn()}
      />,
    );
    const textbox = screen.getByRole("textbox") as HTMLTextAreaElement;
    expect(textbox.disabled).toBe(true);
    expect(textbox.placeholder).toContain("Another session");
  });
});
