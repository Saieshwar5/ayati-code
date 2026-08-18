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
  branch: "perpetual/change",
  create_branch: false,
	authority: "develop",
	effective_mount_mode: "rw",
  preparation_stage: "ready",
  configuration_candidates: [],
  setup_command: "go mod download",
  path: "/workspace",
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
  name: "Perpetual",
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
              id: "builtin-ayati", name: "Perpetual", emoji: "✦", revision: 1,
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
    expect(screen.getByText("Perpetual")).toBeTruthy();
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

  it("creates a task draft from the conversation without sending an agent message", async () => {
    const onCreateTask = vi.fn();
    const onSend = vi.fn();
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
        agents={[builtInAgent]}
        onSend={onSend}
        onStop={vi.fn()}
        onSelectAgent={vi.fn()}
        onCreateTask={onCreateTask}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Create task" }));
    expect(screen.getByRole("button", { name: "Task mode" }).getAttribute("aria-pressed")).toBe("true");
    await user.type(screen.getByRole("textbox"), "Add context compaction");
    await user.click(screen.getByRole("button", { name: "Create task" }));
    expect(onCreateTask).toHaveBeenCalledWith("Add context compaction");
    expect(onSend).not.toHaveBeenCalled();
  });

  it("starts fresh and keeps the previous conversation available as read-only history", async () => {
    const user = userEvent.setup();
    render(
      <ChatPane
        workspace={workspace}
        session={session}
        workspaceSessions={[session]}
        messages={[{ role: "user", content: "Improve workspace navigation" }]}
        error=""
        sending={false}
        stopping={false}
        agents={[builtInAgent]}
        onSend={vi.fn()}
        onStop={vi.fn()}
        onSelectAgent={vi.fn()}
      />,
    );

    await user.type(screen.getByRole("textbox"), "Keep this draft");
    expect(document.querySelector(".conversation-heading")?.textContent).not.toContain("Context");
    await user.click(screen.getByRole("button", { name: "Open context controls" }));
    const tray = screen.getByRole("region", { name: "Context controls" });
    expect(screen.getByText(/Runtime connection pending/)).toBeTruthy();
    expect(tray.textContent).toContain("No past conversations");

    await user.click(screen.getByRole("button", { name: /Start fresh conversation/ }));
    expect(screen.queryByRole("region", { name: "Context controls" })).toBeNull();
    expect(screen.queryByText("Improve workspace navigation")).toBeNull();
    expect(screen.getByText(/Fresh conversation started/)).toBeTruthy();
    expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("Keep this draft");

    await user.click(screen.getByRole("button", { name: "Open context controls" }));
    await user.click(screen.getByRole("button", { name: /Improve workspace navigation/ }));
    expect(screen.getAllByText("Improve workspace navigation").length).toBeGreaterThan(0);
    expect(screen.getByText("Viewing history · messages are read-only")).toBeTruthy();
    expect(screen.queryByRole("textbox")).toBeNull();
    await user.click(screen.getByRole("button", { name: "Return to current conversation" }));
    expect(screen.getByText(/Fresh conversation started/)).toBeTruthy();
    expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("Keep this draft");
  });

  it("shows compaction progress inside the current conversation row", async () => {
    const user = userEvent.setup();
    render(
      <ChatPane
        workspace={workspace}
        session={session}
        workspaceSessions={[session]}
        messages={[{ role: "user", content: "Review the current context" }]}
        error=""
        sending={false}
        stopping={false}
        agents={[builtInAgent]}
        onSend={vi.fn()}
        onStop={vi.fn()}
        onSelectAgent={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "Open context controls" }));
    const current = screen.getByLabelText("Current conversation");
    await user.click(screen.getByRole("button", { name: "Compact" }));
    expect(screen.getByRole("button", { name: "Compacting…" })).toBeTruthy();
    expect(current.classList.contains("working")).toBe(true);
    expect(current.querySelector(".context-compact-progress")).toBeTruthy();
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
    expect(textbox.placeholder).toContain("Another conversation");
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
