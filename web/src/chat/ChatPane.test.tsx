import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { AgentDefinition, Workspace, WorkspaceSession } from "../api/contracts";
import { ChatPane } from "./ChatPane";

const workspace: Workspace = {
  id: "workspace-1",
  repository: "owner/project",
  clone_url: "https://github.com/owner/project.git",
  base_branch: "main",
  branch: "ayati/change",
  create_branch: false,
	authority: "develop",
	effective_mount_mode: "rw",
  preparation_stage: "ready",
  configuration_candidates: [],
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
  selected_agent_id: "builtin-ayati",
  created_at: "2026-08-16T00:00:00Z",
  updated_at: "2026-08-16T00:00:00Z",
};

const builtInAgent: AgentDefinition = {
  id: "builtin-ayati",
  name: "Ayati",
  emoji: "✦",
  description: "General coding agent",
  provider_id: "fireworks",
  model: "",
  max_steps: 20,
  shell_enabled: true,
  instructions: "",
  skill_ids: [],
  revision: 1,
  built_in: true,
  default: true,
  created_at: "2026-08-16T00:00:00Z",
  updated_at: "2026-08-16T00:00:00Z",
};

const reviewerAgent: AgentDefinition = {
  ...builtInAgent,
  id: "reviewer",
  name: "Reviewer",
  emoji: "🔍",
  description: "Reviews changes",
  max_steps: 8,
  built_in: false,
  default: false,
  skill_ids: [],
};

describe("ChatPane", () => {
  it("renders assistant Markdown while keeping user messages literal", () => {
    const { container } = render(
      <ChatPane
        workspace={workspace}
        session={session}
        workspaceSessions={[session]}
        messages={[
          { role: "user", content: "# User request" },
          { role: "assistant", content: "# Agent result\n\n**Complete.**" },
        ]}
        error=""
        sending={false}
        stopping={false}
        agents={[builtInAgent]}
        onSend={vi.fn()}
        onStop={vi.fn()}
        onSelectAgent={vi.fn()}
      />,
    );

    expect(screen.getByRole("heading", { name: "Agent result", level: 1 })).toBeTruthy();
    expect(screen.getByText("Complete.").tagName).toBe("STRONG");
    expect(container.querySelector(".message.user h1")).toBeNull();
    expect(screen.getByText("# User request")).toBeTruthy();
  });

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
          {
            role: "assistant",
            content: "The project is ready.",
            agent: {
              id: "builtin-ayati", name: "Ayati", emoji: "✦", revision: 1,
              provider_id: "fireworks", model: "test-model",
            },
          },
        ]}
        error=""
        sending={false}
        stopping={false}
        agents={[builtInAgent]}
        onSend={send}
        onStop={vi.fn()}
        onSelectAgent={vi.fn()}
      />,
    );

    expect(screen.getByText("Please inspect this.")).toBeTruthy();
    expect(screen.getByText("The project is ready.")).toBeTruthy();
    expect(screen.getByText("Ayati")).toBeTruthy();
    expect(screen.queryByText("shell")).toBeNull();
    await user.type(screen.getByRole("textbox"), "Implement the change");
    await user.click(screen.getByRole("button", { name: "Send message" }));
    expect(send).toHaveBeenCalledWith("Implement the change");
    expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("");
  });

  it("persists a different agent from the composer selector", async () => {
    const selectAgent = vi.fn().mockResolvedValue(undefined);
    const user = userEvent.setup();
    render(
      <ChatPane
        workspace={workspace}
        session={session}
        workspaceSessions={[session]}
        messages={[]}
        error=""
        sending={false}
        stopping={false}
        agents={[builtInAgent, reviewerAgent]}
        onSend={vi.fn()}
        onStop={vi.fn()}
        onSelectAgent={selectAgent}
      />,
    );
    await user.selectOptions(screen.getByRole("combobox", { name: "Agent" }), reviewerAgent.id);
    expect(selectAgent).toHaveBeenCalledWith(reviewerAgent.id);
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
        stopping={false}
        agents={[builtInAgent]}
        onSend={vi.fn()}
        onStop={vi.fn()}
        onSelectAgent={vi.fn()}
      />,
    );
    const textbox = screen.getByRole("textbox") as HTMLTextAreaElement;
    expect(textbox.disabled).toBe(true);
    expect(textbox.placeholder).toContain("Another session");
  });

  it("lets the user stop the active session run", async () => {
    const stop = vi.fn().mockResolvedValue(true);
    const user = userEvent.setup();
    const working = { ...session, status: "working" as const };
    render(
      <ChatPane
        workspace={workspace}
        session={working}
        workspaceSessions={[working]}
        messages={[]}
        error=""
        sending={true}
        stopping={false}
        agents={[builtInAgent]}
        onSend={vi.fn()}
        onStop={stop}
        onSelectAgent={vi.fn()}
      />,
    );

    expect(screen.queryByRole("button", { name: "Send message" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Stop agent run" }));
    expect(stop).toHaveBeenCalledOnce();
  });
});
