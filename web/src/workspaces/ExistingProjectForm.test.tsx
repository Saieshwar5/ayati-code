import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CreateWorkspaceInput, Repository } from "../api/contracts";
import { ExistingProjectForm } from "./ExistingProjectForm";

afterEach(() => vi.restoreAllMocks());

describe("ExistingProjectForm branch selection", () => {
  it("keeps one final creation action after setup without a duplicate summary", () => {
    const onCreate = vi.fn<(input: CreateWorkspaceInput) => Promise<void>>().mockResolvedValue();
    renderForm(onCreate);

    const setup = screen.getByRole("heading", { name: "Setup" });
    const create = screen.getByRole("button", { name: "Create workspace" });

    expect(screen.queryByRole("complementary", { name: "Workspace summary" })).toBeNull();
    expect(setup.compareDocumentPosition(create) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("continues an existing branch with a separate pull request base", async () => {
    const onCreate = vi.fn<(input: CreateWorkspaceInput) => Promise<void>>().mockResolvedValue();
    mockBranches();
    const user = userEvent.setup();
    renderForm(onCreate);

    expect((screen.getByRole("button", { name: "Create workspace" }) as HTMLButtonElement).disabled).toBe(true);
    await user.click(screen.getByRole("radio", { name: "owner/project" }));
    await waitFor(() => expect((screen.getByLabelText("Branch to inspect") as HTMLSelectElement).value).toBe("main"));
    expect((screen.getByRole("button", { name: "Create workspace" }) as HTMLButtonElement).disabled).toBe(false);
    await user.click(screen.getByRole("radio", { name: "Develop authority" }));
    await user.click(screen.getByRole("radio", { name: "Use existing branch" }));
    await user.selectOptions(screen.getByLabelText("Working branch"), "feature/existing");
    expect(screen.getByText(/pull requests will target/i).textContent).toContain("main");
    await user.click(screen.getByRole("button", { name: "Create workspace" }));

    expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      base_branch: "main",
      branch: "feature/existing",
      branch_mode: "existing",
      create_branch: false,
    }));
  });

  it("maps the repository default to direct work and explains the publishing limit", async () => {
    const onCreate = vi.fn<(input: CreateWorkspaceInput) => Promise<void>>().mockResolvedValue();
    mockBranches();
    const user = userEvent.setup();
    renderForm(onCreate);

    await user.click(screen.getByRole("radio", { name: "owner/project" }));
    await screen.findByLabelText("Branch to inspect");
    await user.click(screen.getByRole("radio", { name: "Develop authority" }));
    await user.click(screen.getByRole("radio", { name: "Use existing branch" }));
    await user.selectOptions(screen.getByLabelText("Working branch"), "main");
    expect(screen.getByText(/pull request cannot be created/i)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Create workspace" }));

    expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      base_branch: "main",
      branch: "main",
      branch_mode: "direct",
      create_branch: false,
    }));
  });

  it("filters the repository list before selection", async () => {
    const user = userEvent.setup();
    renderForm(vi.fn().mockResolvedValue(undefined));

    await user.type(screen.getByRole("searchbox", { name: "Search repositories" }), "missing");
    expect(screen.queryByRole("radio", { name: "owner/project" })).toBeNull();
    expect(screen.getByText(/no repositories match/i)).toBeTruthy();
  });

  it("shows only recent suggestions and browses all repositories on demand", async () => {
    const user = userEvent.setup();
    const repositories = Array.from({ length: 7 }, (_, index): Repository => ({
      id: index + 1,
      full_name: `owner/project-${index + 1}`,
      clone_url: `https://github.com/owner/project-${index + 1}.git`,
      default_branch: "main",
      private: true,
    }));
    renderForm(vi.fn().mockResolvedValue(undefined), repositories);

    expect(screen.getAllByRole("radio", { name: /owner\/project-/ })).toHaveLength(5);
    expect(screen.queryByRole("radio", { name: "owner/project-7" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Browse all" }));
    expect(screen.getByRole("dialog", { name: "All repositories" })).toBeTruthy();
    await user.type(screen.getByRole("searchbox", { name: "Search all repositories" }), "project-7");
    await user.click(screen.getByRole("radio", { name: "owner/project-7" }));

    expect(screen.getByRole("complementary", { name: "Selected repository" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Change" })).toBeTruthy();
  });
});

function renderForm(onCreate: (input: CreateWorkspaceInput) => Promise<void>, repositories: Repository[] = [{
  id: 1,
  full_name: "owner/project",
  clone_url: "https://github.com/owner/project.git",
  default_branch: "main",
  private: true,
}]) {
  render(
    <ExistingProjectForm
      repositories={repositories}
      repositoryError=""
      repositoryReconnectRequired={false}
      onCreate={onCreate}
    />,
  );
}

function mockBranches() {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(JSON.stringify([
      { name: "main", commit: { sha: "abc" } },
      { name: "feature/existing", commit: { sha: "def" } },
    ]), { status: 200, headers: { "Content-Type": "application/json" } }),
  );
}
