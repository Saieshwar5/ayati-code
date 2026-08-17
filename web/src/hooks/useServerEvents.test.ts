import { act, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useServerEvents } from "./useServerEvents";

type Listener = (event: Event) => void;

class FakeEventSource {
  static instances: FakeEventSource[] = [];
  readonly listeners = new Map<string, Listener[]>();
  closed = false;

  constructor(readonly url: string) {
    FakeEventSource.instances.push(this);
  }

  addEventListener(type: string, listener: Listener) {
    this.listeners.set(type, [...(this.listeners.get(type) || []), listener]);
  }

  close() {
    this.closed = true;
  }

  emit(type: string, data = "") {
    const event = new MessageEvent(type, { data });
    for (const listener of this.listeners.get(type) || []) listener(event);
  }
}

afterEach(() => {
  vi.unstubAllGlobals();
  FakeEventSource.instances = [];
});

it("keeps one event stream and routes session invalidations", () => {
  vi.stubGlobal("EventSource", FakeEventSource);
  const changed = vi.fn();
  const { result, rerender, unmount } = renderHook(
    ({ workspaceID }) => useServerEvents(workspaceID, changed),
    { initialProps: { workspaceID: "workspace-1" } },
  );

  expect(FakeEventSource.instances).toHaveLength(1);
  expect(FakeEventSource.instances[0].url).toBe("/api/events");
  act(() => FakeEventSource.instances[0].emit("session.changed", JSON.stringify({
    workspace_id: "workspace-1", session_id: "session-1", run_id: "run-1",
  })));
  expect(changed).toHaveBeenCalledWith("workspace-1");
  expect(result.current).toMatchObject({
    type: "session.changed", sessionID: "session-1", runID: "run-1",
  });

  rerender({ workspaceID: "workspace-2" });
  expect(FakeEventSource.instances).toHaveLength(1);
  act(() => FakeEventSource.instances[0].emit("connected"));
  expect(changed).toHaveBeenLastCalledWith("workspace-2");
  unmount();
  expect(FakeEventSource.instances[0].closed).toBe(true);
});
