import type { Workspace } from "../api/contracts";
import { repositoryName, statusLabel } from "../app/format";

interface WorkspaceIndexProps {
  workspaces: Workspace[];
  onCreate: () => void;
  onOpen: (workspaceID: string) => void;
}

export function WorkspaceIndex({ workspaces, onCreate, onOpen }: WorkspaceIndexProps) {
  return (
    <section className="page-scroll">
      <div className="page-frame">
        <header className="page-title-row">
          <div>
            <p className="eyebrow">Projects and sandboxes</p>
            <h1>Workspaces</h1>
            <p className="muted">Open a prepared project, then choose the session you want to continue.</p>
          </div>
          <button className="primary" type="button" onClick={onCreate}>＋ New workspace</button>
        </header>
        {workspaces.length ? (
          <div className="workspace-card-grid">
            {workspaces.map((workspace) => (
              <button className="workspace-card" type="button" key={workspace.id} onClick={() => onOpen(workspace.id)}>
                <span className={`workspace-card-mark ${workspace.status}`} aria-hidden="true" />
                <span className="workspace-card-copy">
                  <strong>{repositoryName(workspace.repository)}</strong>
                  <span>{workspace.repository}</span>
                </span>
                <span className="workspace-card-meta">
                  <span>{workspace.branch}</span>
                  <span className={`status ${workspace.status}`}>{statusLabel(workspace.status)}</span>
                </span>
              </button>
            ))}
          </div>
        ) : (
          <div className="workspace-empty-card">
            <span className="empty-glyph" aria-hidden="true">◇</span>
            <h2>Create your first workspace</h2>
            <p className="muted">Connect a repository, prepare its dependencies, and keep its sessions together.</p>
            <button className="primary" type="button" onClick={onCreate}>Create workspace</button>
          </div>
        )}
      </div>
    </section>
  );
}
