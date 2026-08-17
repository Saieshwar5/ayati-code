import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { MarkdownMessage } from "./MarkdownMessage";

it("renders CommonMark and GitHub-flavored Markdown", () => {
  render(
    <MarkdownMessage content={`# Result

This is **ready** with \`inline code\`.

- first
- second

| File | State |
| --- | --- |
| app.ts | changed |

- [x] Tests passed

\`\`\`ts
const ready = true;
\`\`\``} />,
  );

  expect(screen.getByRole("heading", { name: "Result", level: 1 })).toBeTruthy();
  expect(screen.getByText("ready").tagName).toBe("STRONG");
  expect(screen.getByRole("table")).toBeTruthy();
  expect((screen.getByRole("checkbox") as HTMLInputElement).checked).toBe(true);
  expect(screen.getByText("const ready = true;").tagName).toBe("CODE");
});

it("ignores raw HTML and blocks unsafe link destinations", () => {
  const { container } = render(
    <MarkdownMessage content={`<script>window.compromised = true</script>

[Unsafe](javascript:alert('no')) and [safe](https://example.com).

![tracking pixel](https://tracker.example/pixel.png)`} />,
  );

  expect(container.querySelector("script")).toBeNull();
  const unsafe = screen.getByText("Unsafe").closest("a");
  expect(unsafe).not.toBeNull();
  if (!unsafe) throw new Error("unsafe link was not rendered");
  expect(unsafe.getAttribute("href") || "").not.toMatch(/^javascript:/i);
  const safe = screen.getByRole("link", { name: "safe" });
  expect(safe.getAttribute("href")).toBe("https://example.com");
  expect(safe.getAttribute("target")).toBe("_blank");
  expect(safe.getAttribute("rel")).toBe("noopener noreferrer");
  expect(container.querySelector("img")).toBeNull();
  expect(screen.getByText("[Image omitted: tracking pixel]")).toBeTruthy();
});
