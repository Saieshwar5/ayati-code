import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { useAppRoute, workspaceConversationPath, workspacePath } from "./useAppRoute";

beforeEach(() => window.history.replaceState({}, "", "/workspaces"));

describe("workspace routes", () => {
  it("keeps overview and conversation as distinct destinations", () => {
    const { result } = renderHook(() => useAppRoute());

    act(() => result.current.navigate(workspacePath("workspace/1")));
    expect(result.current.route).toEqual({ page: "workspace-overview", workspaceID: "workspace/1" });

    act(() => result.current.navigate(workspaceConversationPath("workspace/1")));
    expect(result.current.route).toEqual({ page: "workspace-conversation", workspaceID: "workspace/1" });
  });
});
