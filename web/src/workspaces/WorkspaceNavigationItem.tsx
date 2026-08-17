import type { Workspace } from "../api/contracts";
import { repositoryName, statusLabel } from "../app/format";

interface WorkspaceNavigationItemProps {
  workspace: Workspace;
  active: boolean;
  onOpen: () => void;
}

export function WorkspaceNavigationItem({ workspace, active, onOpen }: WorkspaceNavigationItemProps) {
  return (
    <button
      className={`workspace-link ${workspace.status}${active ? " active" : ""}`}
      type="button"
      title={`${workspace.repository} · ${workspace.branch}`}
      aria-current={active ? "page" : undefined}
      onClick={onOpen}
    >
      <span className="workspace-status-dot" aria-hidden="true" />
      <span className="workspace-copy">
        <strong>{repositoryName(workspace.repository)}</strong>
        <span>{workspace.branch}</span>
      </span>
      <span className="sr-only">{statusLabel(workspace.status)}</span>
    </button>
  );
}
