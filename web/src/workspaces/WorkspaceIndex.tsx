import { useMemo, useState } from "react";
import type { Workspace, WorkspaceStatus } from "../api/contracts";
import { repositoryName } from "../app/format";
import { Icon } from "../ui/Icon";

type WorkspaceView = "active" | "archived";
type WorkspaceFilter = "all" | "ready" | "preparing" | "attention" | "stopped";
type WorkspaceSort = "updated" | "name" | "created";

interface WorkspaceIndexProps {
  workspaces: Workspace[];
  archivedWorkspaces: Workspace[];
  view: WorkspaceView;
  onViewChange: (view: WorkspaceView) => void;
  onCreate: () => void;
  onOpen: (workspaceID: string) => void;
  onStop: (workspace: Workspace) => Promise<void>;
  onResume: (workspace: Workspace) => Promise<void>;
  onArchive: (workspace: Workspace) => Promise<boolean>;
  onRestore: (workspace: Workspace) => Promise<void>;
  onDelete: (workspace: Workspace) => Promise<boolean>;
  deletingWorkspaceIDs?: ReadonlySet<string>;
}

export function WorkspaceIndex(props: WorkspaceIndexProps) {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<WorkspaceFilter>("all");
  const [sort, setSort] = useState<WorkspaceSort>("updated");
  const source = props.view === "active" ? props.workspaces : props.archivedWorkspaces;
  const visible = useMemo(
    () => selectWorkspaces(source, query, props.view === "active" ? filter : "all", sort),
    [filter, props.view, query, sort, source],
  );

  return (
    <section className="page-scroll workspace-index">
      <div className="workspace-manager">
        <header className="workspace-manager-header">
          <div>
            <h1>Workspaces</h1>
            <p>{props.workspaces.length} active · {props.archivedWorkspaces.length} archived</p>
          </div>
          <button className="primary compact-action" type="button" onClick={props.onCreate}>
            <Icon name="plus" />
            <span>New workspace</span>
          </button>
        </header>

        <div className="workspace-manager-toolbar">
          <fieldset className="workspace-view-toggle">
            <legend className="sr-only">Workspace view</legend>
            <ViewOption label="Active" count={props.workspaces.length} value="active" selected={props.view} onChange={props.onViewChange} />
            <ViewOption label="Archived" count={props.archivedWorkspaces.length} value="archived" selected={props.view} onChange={props.onViewChange} />
          </fieldset>
          <label className="workspace-search">
            <span className="sr-only">Search workspaces</span>
            <input type="search" value={query} placeholder="Search repository or branch" onChange={(event) => setQuery(event.target.value)} />
          </label>
          {props.view === "active" && (
            <label className="workspace-filter">
              <span className="sr-only">Filter workspaces</span>
              <select value={filter} aria-label="Filter workspaces" onChange={(event) => setFilter(event.target.value as WorkspaceFilter)}>
                <option value="all">All states</option>
                <option value="ready">Ready</option>
                <option value="preparing">Preparing</option>
                <option value="attention">Needs attention</option>
                <option value="stopped">Stopped</option>
              </select>
            </label>
          )}
          <label className="workspace-sort">
            <span className="sr-only">Sort workspaces</span>
            <select value={sort} aria-label="Sort workspaces" onChange={(event) => setSort(event.target.value as WorkspaceSort)}>
              <option value="updated">Recently updated</option>
              <option value="name">Repository name</option>
              <option value="created">Recently created</option>
            </select>
          </label>
        </div>

        {visible.length ? (
          <div className="workspace-table" aria-label={`${props.view === "active" ? "Active" : "Archived"} workspaces`}>
            <div className="workspace-table-header" aria-hidden="true">
              <span>Workspace</span><span>Updated</span><span>Status</span><span />
            </div>
            {visible.map((workspace) => (
              <WorkspaceRow
                key={workspace.id}
                workspace={workspace}
                archived={props.view === "archived"}
                onOpen={() => props.onOpen(workspace.id)}
                onStop={() => props.onStop(workspace)}
                onResume={() => props.onResume(workspace)}
                onArchive={() => props.onArchive(workspace)}
                onRestore={() => props.onRestore(workspace)}
                onDelete={() => props.onDelete(workspace)}
                deleting={props.deletingWorkspaceIDs?.has(workspace.id) || workspace.status === "deleting"}
              />
            ))}
          </div>
        ) : (
          <WorkspaceEmpty view={props.view} hasWorkspaces={source.length > 0} onCreate={props.onCreate} />
        )}
      </div>
    </section>
  );
}

function ViewOption(props: { label: string; count: number; value: WorkspaceView; selected: WorkspaceView; onChange: (view: WorkspaceView) => void }) {
  return (
    <label className={props.selected === props.value ? "selected" : ""}>
      <input type="radio" name="workspace-view" value={props.value} checked={props.selected === props.value} onChange={() => props.onChange(props.value)} />
      <span>{props.label}</span><em>{props.count}</em>
    </label>
  );
}

function WorkspaceRow(props: {
  workspace: Workspace;
  archived: boolean;
  onOpen: () => void;
  onStop: () => Promise<void>;
  onResume: () => Promise<void>;
  onArchive: () => Promise<boolean>;
  onRestore: () => Promise<void>;
  onDelete: () => Promise<boolean>;
  deleting: boolean;
}) {
  const workspace = props.workspace;
  const action = workspace.status === "ready" ? "stop" : workspace.status === "stopped" ? "resume" : "";
  return (
    <article className="workspace-table-row">
      {props.archived ? (
        <div className="workspace-row-identity">
          <WorkspaceIdentity workspace={workspace} />
        </div>
      ) : (
        <button className="workspace-row-open" type="button" disabled={props.deleting} onClick={props.onOpen}>
          <WorkspaceIdentity workspace={workspace} />
          <span className="sr-only">Open workspace</span>
        </button>
      )}
      <time className="workspace-updated" dateTime={workspace.updated_at}>{relativeTime(workspace.updated_at)}</time>
      <span className={`workspace-state ${workspace.status}`}><i aria-hidden="true" />{workspaceStateLabel(workspace.status)}</span>
      <div className="workspace-row-actions">
        {workspace.pull_request_url && !props.archived && (
          <a className="workspace-pr-link" href={workspace.pull_request_url} target="_blank" rel="noreferrer" aria-label={`Open pull request ${workspace.pull_request_number}`}>PR #{workspace.pull_request_number}</a>
        )}
        {props.archived && workspace.status !== "deletion_failed" ? (
          <button className="quiet compact workspace-lifecycle-action" type="button" disabled={props.deleting} onClick={() => void props.onRestore()}>Restore</button>
        ) : action ? (
          <button className="quiet compact workspace-lifecycle-action" type="button" disabled={props.deleting} onClick={() => void (action === "stop" ? props.onStop() : props.onResume())}>
            {action === "stop" ? "Stop" : "Resume"}
          </button>
        ) : null}
        <details className="workspace-row-menu">
          <summary aria-label={`More actions for ${repositoryName(workspace.repository)}`}>•••</summary>
          <div>
            {props.archived || workspace.status === "deletion_failed" ? (
              <button className="danger-text" type="button" disabled={props.deleting} onClick={() => void props.onDelete()}>{props.deleting ? "Deleting…" : workspace.status === "deletion_failed" ? "Retry deletion…" : "Delete workspace…"}</button>
            ) : (
              <button type="button" onClick={() => void props.onArchive()}>Archive workspace…</button>
            )}
          </div>
        </details>
      </div>
    </article>
  );
}

function WorkspaceIdentity({ workspace }: { workspace: Workspace }) {
  return (
    <span className="workspace-card-copy">
      <strong>{repositoryName(workspace.repository)}</strong>
      <span>{workspace.repository} · <code>{workspace.branch}</code></span>
    </span>
  );
}

function WorkspaceEmpty(props: { view: WorkspaceView; hasWorkspaces: boolean; onCreate: () => void }) {
  if (props.hasWorkspaces) {
    return <div className="workspace-empty-card compact-empty"><h2>No matching workspaces</h2><p>Try another search or filter.</p></div>;
  }
  if (props.view === "archived") {
    return <div className="workspace-empty-card compact-empty"><h2>No archived workspaces</h2><p>Archived workspaces remain recoverable here.</p></div>;
  }
  return (
    <div className="workspace-empty-card compact-empty">
      <h2>Create your first workspace</h2>
      <p>Connect a repository and prepare a persistent development environment.</p>
      <button className="primary compact-action" type="button" onClick={props.onCreate}><Icon name="plus" /><span>Create workspace</span></button>
    </div>
  );
}

function selectWorkspaces(values: Workspace[], query: string, filter: WorkspaceFilter, sort: WorkspaceSort): Workspace[] {
  const normalized = query.trim().toLowerCase();
  return values
    .filter((workspace) => !normalized || `${workspace.repository} ${workspace.branch}`.toLowerCase().includes(normalized))
    .filter((workspace) => filter === "all" || workspaceGroup(workspace.status) === filter)
    .sort((left, right) => {
      if (sort === "name") return left.repository.localeCompare(right.repository) || left.branch.localeCompare(right.branch);
      const field = sort === "created" ? "created_at" : "updated_at";
      return Date.parse(right[field]) - Date.parse(left[field]);
    });
}

function workspaceGroup(status: WorkspaceStatus): WorkspaceFilter {
  if (status === "creating" || status === "initializing" || status === "deleting") return "preparing";
  if (status === "needs_configuration" || status === "initialization_failed" || status === "deletion_failed") return "attention";
  return status;
}

function workspaceStateLabel(status: WorkspaceStatus): string {
  if (status === "creating" || status === "initializing") return "Preparing";
  if (status === "needs_configuration") return "Needs input";
  if (status === "initialization_failed") return "Needs attention";
  if (status === "deleting") return "Deleting";
  if (status === "deletion_failed") return "Deletion failed";
  return status[0].toUpperCase() + status.slice(1);
}

function relativeTime(value: string, now = Date.now()): string {
  const elapsed = Math.max(0, now - Date.parse(value));
  if (elapsed < 60_000) return "Just now";
  if (elapsed < 3_600_000) return `${Math.floor(elapsed / 60_000)}m ago`;
  if (elapsed < 86_400_000) return `${Math.floor(elapsed / 3_600_000)}h ago`;
  return new Date(value).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}
