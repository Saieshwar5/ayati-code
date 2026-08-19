import type { Workspace } from "../api/contracts";
import { repositoryName, statusLabel } from "../app/format";
import { Icon } from "../ui/Icon";

interface WorkspaceNavigationItemProps {
  workspace: Workspace;
  active: boolean;
  onOpenConversation: () => void;
  onOpenOverview: () => void;
}

export function WorkspaceNavigationItem({ workspace, active, onOpenConversation, onOpenOverview }: WorkspaceNavigationItemProps) {
  const ready = workspace.status === "ready";
  return (
    <div className={`workspace-link ${workspace.status}${active ? " active" : ""}`}>
      <button
        className="workspace-link-main"
        type="button"
        title={`${ready ? "Continue" : "Open"} ${workspace.repository} · ${workspace.branch}`}
        aria-label={ready ? `Continue ${repositoryName(workspace.repository)} conversation` : `Open ${repositoryName(workspace.repository)} workspace`}
        aria-current={active ? "page" : undefined}
        disabled={workspace.status === "deleting"}
        onClick={ready ? onOpenConversation : onOpenOverview}
      >
        <span className="workspace-status-dot" aria-hidden="true" />
        <span className="workspace-copy">
          <strong>{repositoryName(workspace.repository)}</strong>
          <span>{workspace.branch}</span>
        </span>
        <span className="sr-only">{ready ? "Continue conversation" : `Open workspace · ${statusLabel(workspace.status)}`}</span>
      </button>
      <button className="workspace-link-details" type="button" aria-label={`View ${repositoryName(workspace.repository)} workspace details`} onClick={onOpenOverview}>
        <Icon name="details" />
      </button>
    </div>
  );
}
