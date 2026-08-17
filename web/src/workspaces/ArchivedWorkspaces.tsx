import type { Workspace } from "../api/contracts";
import { repositoryName } from "../app/format";

interface ArchivedWorkspacesProps {
  workspaces: Workspace[];
  onRestore: (workspaceID: string) => Promise<void>;
}

export function ArchivedWorkspaces({ workspaces, onRestore }: ArchivedWorkspacesProps) {
  return (
    <section className="page-scroll">
      <div className="page-frame narrow">
        <header className="page-title-row">
          <div>
            <p className="eyebrow">Recoverable storage</p>
            <h1>Archived workspaces</h1>
            <p className="muted">Archived repositories and sessions are preserved but their sandbox is stopped.</p>
          </div>
        </header>
        <div className="archive-list">
          {workspaces.length ? workspaces.map((workspace) => (
            <article className="archive-card" key={workspace.id}>
              <div>
                <h2>{repositoryName(workspace.repository)}</h2>
                <p>{workspace.repository} · {workspace.branch}</p>
              </div>
              <button className="quiet" type="button" onClick={() => void onRestore(workspace.id)}>Restore</button>
            </article>
          )) : (
            <div className="workspace-empty-card compact-empty">
              <span className="empty-glyph" aria-hidden="true">▱</span>
              <h2>No archived workspaces</h2>
              <p className="muted">Workspaces you archive will remain recoverable here.</p>
            </div>
          )}
        </div>
      </div>
    </section>
  );
}
