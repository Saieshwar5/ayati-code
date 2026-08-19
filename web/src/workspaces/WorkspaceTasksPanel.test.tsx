import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { taskMarkdownFromRequest, WorkspaceTasksPanel, type WorkspaceTask } from "./WorkspaceTasksPanel";

const task: WorkspaceTask = {
  id: "task-1",
  markdown: "# Improve review UI\n\n## Goal\n\nShow a file diff.",
  status: "ready",
};

describe("WorkspaceTasksPanel", () => {
  it("creates a Markdown task without leaving the workspace", async () => {
    const onCreate = vi.fn();
    const user = userEvent.setup();
    renderPanel({ onCreate });

    await user.click(screen.getByLabelText("Create task"));
    const editor = screen.getByLabelText("Task Markdown");
    await user.clear(editor);
    await user.type(editor, "# Add context controls\n\n## Goal\n\nKeep context manageable.");
    await user.click(screen.getByRole("button", { name: "Save task" }));

    expect(onCreate).toHaveBeenCalledWith(expect.stringContaining("# Add context controls"));
  });

  it("edits, previews, and deletes an existing task inline", async () => {
    const onUpdate = vi.fn();
    const onDelete = vi.fn();
    const user = userEvent.setup();
    renderPanel({ tasks: [task], onUpdate, onDelete });

    await user.click(screen.getByRole("button", { name: /Improve review UI.*Ready/i }));
    await user.click(screen.getByRole("button", { name: "Preview" }));
    expect(screen.getByText(/Show a file diff/)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Edit" }));
    await user.type(screen.getByLabelText("Task Markdown"), "\n\n## Verification\n\n- Review the diff");
    await user.click(screen.getByRole("button", { name: "Save task" }));
    expect(onUpdate).toHaveBeenCalledWith(expect.objectContaining({ id: "task-1", markdown: expect.stringContaining("Verification") }));

    await user.click(screen.getByRole("button", { name: /Improve review UI.*Ready/i }));
    await user.click(screen.getByRole("button", { name: "Delete" }));
    expect(onDelete).toHaveBeenCalledWith("task-1");
  });

  it("turns task-mode intent into the shared Markdown template", () => {
    const markdown = taskMarkdownFromRequest("Add automatic context compaction.");
    expect(markdown).toContain("# Add automatic context compaction");
    expect(markdown).toContain("## Requirements");
    expect(markdown).toContain("## Verification");
  });
});

function renderPanel(overrides: Partial<React.ComponentProps<typeof WorkspaceTasksPanel>> = {}) {
  const props: React.ComponentProps<typeof WorkspaceTasksPanel> = {
    tasks: [],
    onCreate: vi.fn(),
    onUpdate: vi.fn(),
    onDelete: vi.fn(),
    ...overrides,
  };
  return render(<WorkspaceTasksPanel {...props} />);
}
