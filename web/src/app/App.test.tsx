import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";

afterEach(() => vi.restoreAllMocks());

describe("App", () => {
  it("explains the GitHub configuration requirement", async () => {
    mockSession({ github_configured: false, authenticated: false });
    render(<App />);
    expect(await screen.findByRole("heading", { name: "Connect Perpetual to GitHub" })).toBeTruthy();
    expect(screen.getByText("PERPETUAL_GITHUB_CLIENT_ID")).toBeTruthy();
  });

  it("shows GitHub login when the app is configured", async () => {
    mockSession({ github_configured: true, authenticated: false });
    render(<App />);
    const link = await screen.findByRole("link", { name: "Continue with GitHub" });
    expect(link.getAttribute("href")).toBe("/auth/github");
  });
});

function mockSession(body: object) {
  vi.spyOn(globalThis, "fetch").mockResolvedValue(
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
}
