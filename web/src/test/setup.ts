import { cleanup } from "@testing-library/react";
import { afterEach } from "vitest";

afterEach(cleanup);

// JSDOM's EventSource is unreliable and can leave open handles. Use a minimal
// stub: listeners are stored, close() cleans up, and no network is attempted.
class EventSourceStub {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSED = 2;
  readonly url: string;
  readyState = EventSourceStub.CONNECTING;
  onopen: ((event: unknown) => void) | null = null;
  onerror: ((event: unknown) => void) | null = null;
  onmessage: ((event: unknown) => void) | null = null;
  private listeners = new Map<string, Array<(event: unknown) => void>>();

  constructor(url: string) {
    this.url = url;
  }

  addEventListener(type: string, callback: (event: unknown) => void): void {
    const existing = this.listeners.get(type) ?? [];
    existing.push(callback);
    this.listeners.set(type, existing);
  }

  removeEventListener(type: string, callback: (event: unknown) => void): void {
    const existing = this.listeners.get(type) ?? [];
    this.listeners.set(type, existing.filter((entry) => entry !== callback));
  }

  close(): void {
    this.readyState = EventSourceStub.CLOSED;
  }

  dispatch(type: string, event: unknown): void {
    for (const callback of this.listeners.get(type) ?? []) callback(event);
    if (type === "message" && this.onmessage) this.onmessage(event);
    if (type === "open" && this.onopen) this.onopen(event);
    if (type === "error" && this.onerror) this.onerror(event);
  }
}

globalThis.EventSource = EventSourceStub as unknown as typeof EventSource;
