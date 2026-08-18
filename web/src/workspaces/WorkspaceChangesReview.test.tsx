import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { WorkspaceChangesReview } from "./WorkspaceChangesReview";

const changes = {
  status: " M web/src/app.tsx\n?? web/src/new.ts",
  diff: `diff --git a/web/src/app.tsx b/web/src/app.tsx
index 111..222 100644
--- a/web/src/app.tsx
+++ b/web/src/app.tsx
@@ -1 +1 @@
-old
+new
diff --git a/web/src/new.ts b/web/src/new.ts
new file mode 100644
--- /dev/null
+++ b/web/src/new.ts
@@ -0,0 +1 @@
+export {};
`,
};

describe("WorkspaceChangesReview", () => {
  it("turns the current unified diff into a selectable file review", async () => {
    const user = userEvent.setup();
    render(<WorkspaceChangesReview changes={changes} loading={false} onRefresh={vi.fn()} onPublish={vi.fn()} />);

    expect(screen.getByRole("heading", { name: "2 changed files" })).toBeTruthy();
    const files = screen.getByLabelText("Changed files");
    expect(within(files).getByRole("button", { name: /web\/src\/app.tsx/ })).toBeTruthy();
    await user.click(within(files).getByRole("button", { name: /web\/src\/new.ts/ }));
    expect(screen.getByLabelText("Diff for web/src/new.ts").textContent).toContain("export {}");

    await user.type(screen.getByRole("searchbox", { name: "Filter changed files" }), "app.tsx");
    expect(within(files).queryByRole("button", { name: /new.ts/ })).toBeNull();
  });

  it("keeps refresh and publish close to review", async () => {
    const onRefresh = vi.fn();
    const onPublish = vi.fn();
    const user = userEvent.setup();
    const { rerender } = render(<WorkspaceChangesReview changes={{ status: "", diff: "" }} loading={false} onRefresh={onRefresh} onPublish={onPublish} />);

    expect(screen.getByText("Working tree is clean")).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Refresh" }));
    expect((screen.getByRole("button", { name: "Publish…" }) as HTMLButtonElement).disabled).toBe(true);
    rerender(<WorkspaceChangesReview changes={changes} loading={false} onRefresh={onRefresh} onPublish={onPublish} />);
    await user.click(screen.getByRole("button", { name: "Publish…" }));
    expect(onRefresh).toHaveBeenCalledOnce();
    expect(onPublish).toHaveBeenCalledOnce();
  });
});
