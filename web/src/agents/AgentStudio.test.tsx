import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AgentDefinition, SkillDefinition } from "../api/contracts";
import { WorkspaceApplication } from "../app/WorkspaceApplication";

const builtIn: AgentDefinition = {
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
  created_at: "2026-08-17T00:00:00Z",
  updated_at: "2026-08-17T00:00:00Z",
};

const reviewSkill: SkillDefinition = {
  id: "skill-go-review",
  name: "Go review",
  description: "Review Go boundaries",
  markdown: "Check context cancellation.",
  revision: 1,
  attached_agents: 0,
  created_at: "2026-08-17T00:00:00Z",
  updated_at: "2026-08-17T00:00:00Z",
};

afterEach(() => vi.restoreAllMocks());
beforeEach(() => window.history.replaceState({}, "", "/agents"));

describe("AgentStudio", () => {
  it("shows global agent navigation and creates a reusable agent", async () => {
    let created: AgentDefinition | undefined;
    let createRequest: RequestInit | undefined;
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === "/api/repositories") return json([]);
      if (path === "/api/workspaces") return json([]);
      if (path === "/api/workspaces?archived=true") return json([]);
      if (path === "/api/agents?archived=true") return json([]);
      if (path === "/api/skills?archived=true") return json([]);
      if (path === "/api/skills") return json([reviewSkill]);
      if (path === "/api/agents" && init?.method === "POST") {
        createRequest = init;
        const body = JSON.parse(String(init.body));
        created = {
          ...builtIn,
          ...body,
          id: "agent-tests",
          revision: 1,
          built_in: false,
          default: false,
        };
        return json(created, 201);
      }
      if (path === "/api/agents") return json(created ? [builtIn, created] : [builtIn]);
      throw new Error(`Unexpected request: ${init?.method || "GET"} ${path}`);
    });

    const user = userEvent.setup();
    render(<WorkspaceApplication user={{ id: 1, login: "octocat", avatar_url: "avatar.png" }} />);

    const studio = await screen.findByRole("complementary", { name: "Agent Studio navigation" });
    expect(within(studio).getByRole("button", { name: /Agents/ })).toBeTruthy();
    expect(within(studio).getByRole("button", { name: /Providers/ })).toBeTruthy();
    expect(within(studio).getByRole("button", { name: /Skills/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Open Ayati" })).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "＋ New agent" }));
    await user.clear(screen.getByLabelText("Emoji"));
    await user.type(screen.getByLabelText("Emoji"), "🧪");
    await user.type(screen.getByLabelText("Name"), "Test specialist");
    await user.type(screen.getByLabelText("Description"), "Improves tests");
    await user.type(screen.getByLabelText("Model"), "test-model");
    await user.clear(screen.getByLabelText("Step limit"));
    await user.type(screen.getByLabelText("Step limit"), "8");
    await user.type(screen.getByLabelText("Agent instructions"), "Inspect failures first.");
    await user.selectOptions(screen.getByLabelText("Add skill"), reviewSkill.id);
    await user.click(screen.getByRole("button", { name: "Save agent" }));

    expect(await screen.findByRole("heading", { name: "Test specialist" })).toBeTruthy();
    await waitFor(() => expect(createRequest).toBeTruthy());
    expect(JSON.parse(String(createRequest?.body))).toMatchObject({
      name: "Test specialist",
      emoji: "🧪",
      provider_id: "fireworks",
      model: "test-model",
      max_steps: 8,
      shell_enabled: true,
      instructions: "Inspect failures first.",
      skill_ids: [reviewSkill.id],
    });
  });

  it("creates reusable Markdown guidance from the Skills page", async () => {
    window.history.replaceState({}, "", "/agents/skills");
    let saved: SkillDefinition | undefined;
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === "/api/repositories") return json([]);
      if (path === "/api/workspaces") return json([]);
      if (path === "/api/workspaces?archived=true") return json([]);
      if (path === "/api/agents") return json([builtIn]);
      if (path === "/api/agents?archived=true") return json([]);
      if (path === "/api/skills?archived=true") return json([]);
      if (path === "/api/skills" && init?.method === "POST") {
        const body = JSON.parse(String(init.body));
        saved = { ...reviewSkill, ...body, id: "skill-testing" };
        return json(saved, 201);
      }
      if (path === "/api/skills") return json(saved ? [saved] : []);
      throw new Error(`Unexpected request: ${init?.method || "GET"} ${path}`);
    });

    const user = userEvent.setup();
    render(<WorkspaceApplication user={{ id: 1, login: "octocat", avatar_url: "avatar.png" }} />);
    expect(await screen.findByRole("heading", { name: "Skills" })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "＋ New skill" }));
    await user.type(screen.getByLabelText("Name"), "Testing discipline");
    await user.type(screen.getByLabelText("Description"), "Reliable verification");
    await user.type(screen.getByLabelText("Markdown"), "# Testing\n\nRun focused tests first.");
    await user.click(screen.getByRole("button", { name: "Save skill" }));
    expect(await screen.findByRole("button", { name: "Edit Testing discipline" })).toBeTruthy();
  });
});

function json(body: unknown, status = 200) {
  return Promise.resolve(new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  }));
}
