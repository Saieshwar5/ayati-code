import type { WorkspaceSession } from "../api/contracts";

export function statusLabel(status: string): string {
  return status.replaceAll("_", " ");
}

export function repositoryName(repository: string): string {
  return repository.split("/").at(-1) || repository;
}

export function sessionMeta(session: WorkspaceSession, now = Date.now()): string {
  if (session.status === "working") return "Working now";
  if (session.status === "failed") return "Failed";
  if (session.status === "review") return "Review changes";
  const time = new Date(session.updated_at);
  const elapsed = now - time.getTime();
  if (elapsed < 60_000) return "Just now";
  if (elapsed < 3_600_000) return `${Math.floor(elapsed / 60_000)}m ago`;
  if (elapsed < 86_400_000) return `${Math.floor(elapsed / 3_600_000)}h ago`;
  return time.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}
