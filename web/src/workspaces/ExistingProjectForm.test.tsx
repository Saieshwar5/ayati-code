import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { CreateWorkspaceInput } from "../api/contracts";
import { ExistingProjectForm } from "./ExistingProjectForm";

afterEach(() => vi.restoreAllMocks());

describe("ExistingProjectForm branch selection", () => {
  it("continues an existing branch with a separate pull request base", async () => {
    const onCreate = vi.fn<(input: CreateWorkspaceInput) => Promise<void>>().mockResolvedValue();
    mockBranches();
    const user = userEvent.setup();
    renderForm(onCreate);

    await user.selectOptions(screen.getByLabelText("Repository"), "owner/project");
    await waitFor(() => expect((screen.getByLabelText("Branch to inspect") as HTMLSelectElement).value).toBe("main"));
    await user.click(screen.getByRole("radio", { name: "Develop authority" }));
    await user.click(screen.getByRole("radio", { name: /Continue an existing branch/i }));
    await user.selectOptions(screen.getByLabelText("Working branch"), "feature/existing");
    await user.click(screen.getByRole("button", { name: "Create and initialize" }));

    expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      base_branch: "main",
      branch: "feature/existing",
      branch_mode: "existing",
      create_branch: false,
    }));
  });

  it("makes direct branch work explicit and keeps both branch fields equal", async () => {
    const onCreate = vi.fn<(input: CreateWorkspaceInput) => Promise<void>>().mockResolvedValue();
    mockBranches();
    const user = userEvent.setup();
    renderForm(onCreate);

    await user.selectOptions(screen.getByLabelText("Repository"), "owner/project");
    await screen.findByLabelText("Branch to inspect");
    await user.click(screen.getByRole("radio", { name: "Develop authority" }));
    await user.click(screen.getByRole("radio", { name: /Work directly on a branch/i }));
    await user.selectOptions(screen.getByLabelText("Working branch"), "main");
    expect(screen.getByText(/will not push or open a pull request/i)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Create and initialize" }));

    expect(onCreate).toHaveBeenCalledWith(expect.objectContaining({
      base_branch: "main",
      branch: "main",
      branch_mode: "direct",
      create_branch: false,
    }));
  });
});

function renderForm(onCreate: (input: CreateWorkspaceInput) => Promise<void>) {
  render(
    <ExistingProjectForm
      repositories={[{
        id: 1,
        full_name: "owner/project",
        clone_url: "https://github.com/owner/project.git",
        default_branch: "main",
        private: true,
      }]}
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
