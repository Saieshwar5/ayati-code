import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Workspace, WorkspaceSession } from "../api/contracts";
import { WorkspaceApplication } from "./WorkspaceApplication";

const workspace: Workspace = {
  id: "workspace-1",
  repository: "owner/project",
  clone_url: "https://github.com/owner/project.git",
  base_branch: "main",
  branch: "perpetual/react-ui",
  create_branch: false,
  preparation_stage: "cloning",
  preparation_detail: "owner/project · perpetual/react-ui",
  configuration_candidates: [],
  setup_command: "go mod download",
  path: "/workspace",
  status: "creating",
  created_at: "2026-08-16T00:00:00Z",
  updated_at: "2026-08-16T00:00:00Z",
};

const session: WorkspaceSession = {
  id: "session-1",
  workspace_id: workspace.id,
  title: "Original session",
  status: "idle",
  created_at: "2026-08-16T00:00:00Z",
  updated_at: "2026-08-16T00:00:00Z",
};

afterEach(() => vi.restoreAllMocks());
beforeEach(() => {
  window.history.replaceState({}, "", "/workspaces");
  const values = new Map<string, string>();
  Object.defineProperty(window, "localStorage", {
    configurable: true,
    value: {
      clear: () => values.clear(),
      getItem: (key: string) => values.get(key) ?? null,
      key: (index: number) => [...values.keys()][index] ?? null,
      get length() { return values.size; },
      removeItem: (key: string) => values.delete(key),
      setItem: (key: string, value: string) => values.set(key, value),
    } satisfies Storage,
  });
});

describe("WorkspaceApplication", () => {
  it("collapses the main navigation and remembers the preference", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/repositories") return json([]);
      if (path === "/api/workspaces") return json([]);
      if (path === "/api/workspaces?archived=true") return json([]);
      return json([]);
    });

    const user = userEvent.setup();
    render(
      <WorkspaceApplication
        user={{ id: 1, login: "octocat", avatar_url: "https://example.test/avatar.png" }}
      />,
    );

    await screen.findByRole("heading", { name: "Workspaces" });
    const collapse = screen.getByRole("button", { name: "Collapse sidebar" });
    expect(collapse.getAttribute("aria-expanded")).toBe("true");

    await user.click(collapse);

    const expand = screen.getByRole("button", { name: "Expand sidebar" });
    expect(expand.getAttribute("aria-expanded")).toBe("false");
    expect(expand.querySelector(".perpetual-mark-pendulum")).toBeTruthy();
    expect(document.querySelector(".app-shell")?.classList.contains("sidebar-collapsed")).toBe(true);
    expect(window.localStorage.getItem("perpetual.sidebar.collapsed")).toBe("true");

    await user.click(expand);

    expect(screen.getByRole("button", { name: "Collapse sidebar" })).toBeTruthy();
    expect(document.querySelector(".perpetual-mark")).toBeNull();
    expect(window.localStorage.getItem("perpetual.sidebar.collapsed")).toBe("false");

    await user.click(screen.getByRole("button", { name: "Collapse sidebar" }));
    await user.click(screen.getByRole("button", { name: "Create workspace from navigation" }));

    expect(window.location.pathname).toBe("/workspaces/new");
    expect(document.querySelector(".app-shell")?.classList.contains("sidebar-collapsed")).toBe(false);
    expect(screen.getByRole("button", { name: "Collapse sidebar" })).toBeTruthy();

    await user.click(screen.getByRole("button", { name: "Collapse sidebar" }));
    await user.click(screen.getByRole("complementary", { name: "Main navigation" }));

    expect(document.querySelector(".app-shell")?.classList.contains("sidebar-collapsed")).toBe(false);
  });

  it("offers GitHub reconnection when repository authorization expires", async () => {
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/repositories") {
        return json({ error: "GitHub authorization expired; reconnect GitHub" }, 401);
      }
      if (path === "/api/workspaces") return json([]);
      if (path === "/api/workspaces?archived=true") return json([]);
      throw new Error(`Unexpected request: GET ${path}`);
    });

    render(
      <WorkspaceApplication
        user={{ id: 1, login: "octocat", avatar_url: "https://example.test/avatar.png" }}
      />,
    );

    const link = await screen.findByRole("link", { name: "Reconnect GitHub" });
    expect(link.getAttribute("href")).toBe("/auth/github");
    expect(screen.getByText(/authorization expired/i)).toBeTruthy();
  });

  it("creates a workspace from an installed repository", async () => {
    let created = false;
    let createRequest: RequestInit | undefined;
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === "/api/repositories") {
        return json([{ id: 1, full_name: "owner/project", default_branch: "main" }]);
      }
      if (path === "/api/workspaces" && init?.method === "POST") {
        created = true;
        createRequest = init;
        return json(workspace, 202);
      }
      if (path === "/api/workspaces") return json(created ? [workspace] : []);
      if (path === "/api/workspaces?archived=true") return json([]);
      if (path === "/api/repositories/owner/project/branches") {
        return json([{ name: "main", commit: { sha: "abc123" } }]);
      }
      if (path === `/api/workspaces/${workspace.id}/sessions`) return json([session]);
      if (path === `/api/workspaces/${workspace.id}/sessions/${session.id}/messages`) return json([]);
      throw new Error(`Unexpected request: ${init?.method || "GET"} ${path}`);
    });

    const user = userEvent.setup();
    render(
      <WorkspaceApplication
        user={{ id: 1, login: "octocat", avatar_url: "https://example.test/avatar.png" }}
      />,
    );
    await screen.findByRole("heading", { name: "Workspaces" });
    expect(screen.getByRole("button", { name: "perpetual" })).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "New workspace" }));
    await user.click(screen.getByRole("radio", { name: "owner/project" }));
    await screen.findByLabelText("Base branch");
    await user.type(screen.getByLabelText("New branch name"), "perpetual/react-ui");
    await user.click(screen.getByText("Environment variables"));
    await user.click(screen.getByRole("button", { name: "Add variable" }));
    await user.type(screen.getByLabelText("Name"), "NPM_TOKEN");
    await user.type(screen.getByLabelText("Value"), "private-token");
    await user.click(screen.getByLabelText("During setup"));
    await user.click(screen.getByRole("button", { name: "Create workspace" }));

    expect(await screen.findByRole("heading", { name: "project", level: 1 })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Preparing your workspace", level: 2 })).toBeTruthy();
    expect(window.location.pathname).toBe(`/workspaces/${workspace.id}`);
    expect(screen.getByRole("button", { name: "Continue conversation" }).hasAttribute("disabled")).toBe(true);
    expect(new Headers(createRequest?.headers).get("X-Perpetual-Request")).toBe("1");
    expect(JSON.parse(String(createRequest?.body))).toMatchObject({
      repository: "owner/project",
      base_branch: "main",
      branch: "perpetual/react-ui",
      create_branch: true,
      branch_mode: "new",
      environment: [
        { name: "NPM_TOKEN", value: "private-token", expose_during_setup: true },
      ],
    });
  });

  it("defaults new workspaces to a new working branch", async () => {
    let createRequest: RequestInit | undefined;
    const user = userEvent.setup({ delay: null });
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === "/api/repositories") {
        return json([{ id: 1, full_name: "owner/project", default_branch: "main" }]);
      }
      if (path === "/api/workspaces" && init?.method === "POST") {
        createRequest = init;
        return json({ ...workspace, branch: "perpetual/default-flow", create_branch: true }, 202);
      }
      if (path === "/api/workspaces") return json([]);
      if (path === "/api/workspaces?archived=true") return json([]);
      if (path === "/api/repositories/owner/project/branches") {
        return json([{ name: "main", commit: { sha: "abc123" } }]);
      }
      if (path === `/api/workspaces/${workspace.id}/sessions`) return json([session]);
      if (path === `/api/workspaces/${workspace.id}/sessions/${session.id}/messages`) return json([]);
      throw new Error(`Unexpected request: ${init?.method || "GET"} ${path}`);
    });

    render(
      <WorkspaceApplication
        user={{ id: 1, login: "octocat", avatar_url: "https://example.test/avatar.png" }}
      />,
    );
    await screen.findByRole("heading", { name: "Workspaces" });
    await user.click(screen.getByRole("button", { name: "New workspace" }));
    await user.click(screen.getByRole("radio", { name: "owner/project" }));
    await screen.findByLabelText("Base branch");
    expect((screen.getByRole("radio", { name: "Create new branch" }) as HTMLInputElement).checked).toBe(true);
    await user.type(screen.getByLabelText("New branch name"), "perpetual/default-flow");
    await user.click(screen.getByRole("button", { name: "Create workspace" }));

    expect(JSON.parse(String(createRequest?.body))).toMatchObject({
      base_branch: "main",
      branch: "perpetual/default-flow",
      create_branch: true,
      branch_mode: "new",
    });
  });

  it("creates a private GitHub project with a working branch", async () => {
    const createdWorkspace: Workspace = {
      ...workspace,
      repository: "octocat/new-project",
      clone_url: "https://github.com/octocat/new-project.git",
      base_branch: "trunk",
      branch: "perpetual/initial",
      create_branch: true,
    };
    let created = false;
    let createRequest: RequestInit | undefined;
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const path = String(input);
      if (path === "/api/repositories") return json([]);
      if (path === "/api/workspaces/new-project" && init?.method === "POST") {
        created = true;
        createRequest = init;
        return json(createdWorkspace, 202);
      }
      if (path === "/api/workspaces") return json(created ? [createdWorkspace] : []);
      if (path === "/api/workspaces?archived=true") return json([]);
      if (path === `/api/workspaces/${workspace.id}/sessions`) return json([session]);
      if (path === `/api/workspaces/${workspace.id}/sessions/${session.id}/messages`) return json([]);
      throw new Error(`Unexpected request: ${init?.method || "GET"} ${path}`);
    });

    const user = userEvent.setup();
    render(
      <WorkspaceApplication
        user={{ id: 1, login: "octocat", avatar_url: "https://example.test/avatar.png" }}
      />,
    );
    await screen.findByRole("heading", { name: "Workspaces" });
    await user.click(screen.getByRole("button", { name: "New workspace" }));
    await user.click(screen.getByRole("radio", { name: "New project" }));
    await user.type(screen.getByLabelText("Repository name"), "new-project");
    expect((screen.getByRole("radio", { name: "Private" }) as HTMLInputElement).checked).toBe(true);
    await user.click(screen.getByRole("button", { name: "Create workspace" }));

    expect(await screen.findByRole("heading", { name: "new-project", level: 1 })).toBeTruthy();
    expect(JSON.parse(String(createRequest?.body))).toMatchObject({
      name: "new-project",
      private: true,
      branch: "perpetual/initial",
    });
  });

  it("keeps review on the workspace overview and conversation free of a right section", async () => {
    const readyWorkspace: Workspace = {
      ...workspace,
      status: "ready",
      preparation_stage: "ready",
    };
    window.history.replaceState({}, "", `/workspaces/${workspace.id}`);
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/repositories") return json([]);
      if (path === "/api/workspaces") return json([readyWorkspace]);
      if (path === "/api/workspaces?archived=true") return json([]);
      if (path === `/api/workspaces/${workspace.id}/sessions`) return json([session]);
      if (path === `/api/workspaces/${workspace.id}/sessions/${session.id}/messages`) return json([]);
      if (path === `/api/workspaces/${workspace.id}/changes`) return json({ status: "", diff: "" });
      throw new Error(`Unexpected request: GET ${path}`);
    });

    const user = userEvent.setup();
    render(<WorkspaceApplication user={{ id: 1, login: "octocat", avatar_url: "avatar.png" }} />);

    expect(await screen.findByRole("heading", { name: "project", level: 1 })).toBeTruthy();
    expect(window.location.pathname).toBe(`/workspaces/${workspace.id}`);
    expect(screen.getByRole("heading", { name: "Tasks" })).toBeTruthy();
    expect(screen.getByRole("heading", { name: "Changes" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Open context controls" })).toBeNull();
    const sidebar = screen.getByRole("complementary", { name: "Main navigation" });
    expect(within(sidebar).queryByText("Sessions")).toBeNull();

    await user.click(screen.getByRole("button", { name: "Review changes" }));
    expect(screen.getByRole("region", { name: "Workspace changes" })).toBeTruthy();
    expect((screen.getByRole("button", { name: "Publish…" }) as HTMLButtonElement).disabled).toBe(true);

    await user.click(screen.getByRole("button", { name: "Continue conversation" }));
    expect(window.location.pathname).toBe(`/workspaces/${workspace.id}/conversation`);
    expect(await screen.findByRole("button", { name: "Open context controls" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Back to project workspace" })).toBeTruthy();
    expect(screen.queryByRole("complementary", { name: "Workspace changes" })).toBeNull();
    expect(screen.getByRole("button", { name: "Open context controls" })).toBeTruthy();
    const header = document.querySelector(".conversation-heading");
    expect(header?.textContent).not.toContain("Tasks");
    expect(header?.textContent).not.toContain("Changes");
    expect(screen.queryByRole("navigation", { name: "Conversation tools" })).toBeNull();
    expect(screen.queryByRole("region", { name: "Workspace changes" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Changes" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Tasks" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Workspace" })).toBeNull();
  });

  it("continues a ready recent workspace directly while keeping its overview available", async () => {
    const readyWorkspace: Workspace = { ...workspace, status: "ready", preparation_stage: "ready" };
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input) => {
      const path = String(input);
      if (path === "/api/repositories") return json([]);
      if (path === "/api/workspaces") return json([readyWorkspace]);
      if (path === "/api/workspaces?archived=true") return json([]);
      if (path === `/api/workspaces/${workspace.id}/sessions`) return json([session]);
      if (path === `/api/workspaces/${workspace.id}/sessions/${session.id}/messages`) return json([]);
      throw new Error(`Unexpected request: GET ${path}`);
    });

    const user = userEvent.setup();
    render(<WorkspaceApplication user={{ id: 1, login: "octocat", avatar_url: "avatar.png" }} />);

    await screen.findByRole("heading", { name: "Workspaces" });
    const sidebar = screen.getByRole("complementary", { name: "Main navigation" });
    await user.click(within(sidebar).getByRole("button", { name: "Continue project conversation" }));
    expect(window.location.pathname).toBe(`/workspaces/${workspace.id}/conversation`);
    expect(await screen.findByRole("button", { name: "Open context controls" })).toBeTruthy();

    await user.click(within(sidebar).getByRole("button", { name: "View project workspace details" }));
    expect(window.location.pathname).toBe(`/workspaces/${workspace.id}`);
    expect(await screen.findByRole("heading", { name: "project", level: 1 })).toBeTruthy();
  });
});

function json(body: unknown, status = 200) {
  return Promise.resolve(
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}
