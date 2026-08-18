import type { Workspace } from "../api/contracts";
import { repositoryName, statusLabel } from "../app/format";
import { Icon } from "../ui/Icon";

interface WorkspaceIndexProps {
  workspaces: Workspace[];
  onCreate: () => void;
  onOpen: (workspaceID: string) => void;
}

export function WorkspaceIndex({ workspaces, onCreate, onOpen }: WorkspaceIndexProps) {
  return (
    <section className="page-scroll workspace-index">
      <div className="page-frame">
        <header className="page-title-row">
          <div>
            <h1>Workspaces</h1>
            <p className="muted">Your repositories, environments, and sessions.</p>
          </div>
          <button className="primary compact-action" type="button" onClick={onCreate}>
            <Icon name="plus" />
            <span>New workspace</span>
          </button>
        </header>
        {workspaces.length ? (
          <div className="workspace-table" aria-label="Workspaces">
            {workspaces.map((workspace) => (
              <button className="workspace-table-row" type="button" key={workspace.id} onClick={() => onOpen(workspace.id)}>
                <span className="workspace-card-copy">
                  <strong>{repositoryName(workspace.repository)}</strong>
                  <span>{workspace.repository}</span>
                </span>
                <code className="workspace-branch">{workspace.branch}</code>
                <span className={`status ${workspace.status}`}>{statusLabel(workspace.status)}</span>
                <span className="row-arrow" aria-hidden="true">›</span>
              </button>
            ))}
          </div>
        ) : (
          <div className="workspace-empty-card">
            <h2>Create your first workspace</h2>
            <p className="muted">Connect a repository and prepare a persistent development environment.</p>
            <button className="primary compact-action" type="button" onClick={onCreate}>
              <Icon name="plus" />
              <span>Create workspace</span>
            </button>
          </div>
        )}
      </div>
    </section>
  );
}
