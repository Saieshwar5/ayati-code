import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { Message, ToolCall } from "../api/contracts";
import { AgentActivity, buildConversationFeed, type ActivityGroup } from "./AgentActivity";

describe("AgentActivity", () => {
  it("groups durable shell calls and results between the request and final answer", () => {
    const feed = buildConversationFeed([
      { id: 1, role: "user", content: "Check the project" },
      { id: 2, role: "assistant", tool_calls: [shellCall("call-1", "rg --files")] },
      { id: 3, role: "tool", tool_call_id: "call-1", content: shellResult("main.go") },
      { id: 4, role: "assistant", content: "The project is ready." },
    ]);

    expect(feed).toHaveLength(3);
    expect(feed[1].kind).toBe("activity");
    const activity = feed[1] as ActivityGroup;
    expect(activity.id).toBe("activity-1");
    expect(activity.closed).toBe(true);
    expect(activity.completed).toBe(true);
    expect(activity.steps[0].result?.id).toBe(3);
  });

  it("opens the newest step, collapses the previous step, and compacts on completion", async () => {
    const user = userEvent.setup();
    const first = step("call-1", "rg --files", shellResult("main.go"));
    const { rerender } = render(
      <AgentActivity group={group([first])} state="working" />,
    );
    const firstButton = screen.getByRole("button", { name: /Step 1 · Search project/ });
    expect(firstButton.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("main.go")).toBeTruthy();

    const second = step("call-2", "go test ./...");
    rerender(<AgentActivity group={group([first, second])} state="working" />);
    const secondButton = screen.getByRole("button", { name: /Step 2 · Run tests/ });
    await waitFor(() => expect(secondButton.getAttribute("aria-expanded")).toBe("true"));
    expect(firstButton.getAttribute("aria-expanded")).toBe("false");
    expect(screen.getByText("Command is running…")).toBeTruthy();

    await user.click(firstButton);
    expect(firstButton.getAttribute("aria-expanded")).toBe("true");
    expect(secondButton.getAttribute("aria-expanded")).toBe("false");

    rerender(<AgentActivity group={{ ...group([first, second]), closed: true, completed: true }} state="completed" />);
    const activityToggle = screen.getByRole("button", { name: /Agent activity: Completed · 2 steps/ });
    await waitFor(() => expect(activityToggle.getAttribute("aria-expanded")).toBe("false"));
    expect(screen.queryByRole("button", { name: /Step 1 · Search project/ })).toBeNull();

    await user.click(activityToggle);
    expect(activityToggle.getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByRole("button", { name: /Step 1 · Search project/ })).toBeTruthy();
  });

  it("keeps a failed current command expanded with its result", () => {
    const failed = step("call-1", "go test ./...", shellResult("FAIL", 1));
    render(<AgentActivity group={group([failed])} state="failed" />);

    expect(screen.getByRole("button", { name: /Failed · exit 1/ }).getAttribute("aria-expanded")).toBe("true");
    expect(screen.getByText("FAIL")).toBeTruthy();
  });
});

function shellCall(id: string, command: string): ToolCall {
  return { id, type: "function", function: { name: "shell", arguments: JSON.stringify({ command }) } };
}

function shellResult(stdout: string, exitCode = 0): string {
  return JSON.stringify({ command: "command", stdout, exit_code: exitCode, duration: 25_000_000 });
}

function step(id: string, command: string, result?: string) {
  return {
    id,
    call: shellCall(id, command),
    result: result ? { id: Number(id.slice(-1)) + 10, role: "tool", tool_call_id: id, content: result } : undefined,
  };
}

function group(steps: ReturnType<typeof step>[]): ActivityGroup {
  return { kind: "activity", id: "activity-request", closed: false, completed: false, steps: steps as ActivityGroup["steps"] };
}
