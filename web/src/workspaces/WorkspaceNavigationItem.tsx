import type { Workspace, WorkspaceSession } from "../api/contracts";
import { repositoryName, sessionMeta, statusLabel } from "../app/format";

interface WorkspaceNavigationItemProps {
  workspace: Workspace;
  sessions: WorkspaceSession[];
  expanded: boolean;
  activeWorkspaceID: string;
  activeSessionID: string;
  onToggle: () => void;
  onOpenSession: (sessionID: string) => void;
  onCreateSession: () => void;
  onRenameSession: (session: WorkspaceSession) => void;
  onDeleteSession: (session: WorkspaceSession) => void;
  onAction: (action: "initialize" | "stop") => void;
  onDelete: () => void;
}

const refreshingStatuses = new Set(["creating", "initializing"]);

export function WorkspaceNavigationItem(props: WorkspaceNavigationItemProps) {
  const { workspace, sessions, expanded } = props;
  const className = [
    "workspace-item",
    workspace.status,
    expanded ? "expanded" : "",
    props.activeWorkspaceID === workspace.id ? "active" : "",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <article className={className}>
      <div className="workspace-row">
        <button
          className="workspace-open"
          type="button"
          aria-expanded={expanded}
          title={`${workspace.repository} · ${workspace.branch}`}
          onClick={props.onToggle}
        >
          <span className="workspace-chevron" aria-hidden="true">
            ›
          </span>
          <span className="workspace-status-dot" aria-hidden="true" />
          <span className="workspace-copy">
            <strong>{repositoryName(workspace.repository)}</strong>
            <span>{workspace.branch}</span>
          </span>
          <span className={`status workspace-status ${workspace.status}`}>
            {statusLabel(workspace.status)}
          </span>
        </button>
        <details className="workspace-menu">
          <summary aria-label="Workspace actions">•••</summary>
          <div className="context-menu">
            <button type="button" onClick={props.onCreateSession}>
              New session
            </button>
            {(["initialization_failed", "stopped"] as const).includes(workspace.status as never) && (
              <button type="button" onClick={() => props.onAction("initialize")}>
                Resume environment
              </button>
            )}
            {(["ready", "initialization_failed"] as const).includes(workspace.status as never) && (
              <button type="button" onClick={() => props.onAction("stop")}>
                Stop environment
              </button>
            )}
            {workspace.pull_request_url && (
              <a href={workspace.pull_request_url} target="_blank" rel="noreferrer">
                Open pull request
              </a>
            )}
            {!refreshingStatuses.has(workspace.status) && (
              <button className="delete-workspace danger" type="button" onClick={props.onDelete}>
                Delete workspace…
              </button>
            )}
          </div>
        </details>
      </div>

      {expanded && (
        <div className="session-navigation">
          <button className="inline-new-session" type="button" onClick={props.onCreateSession}>
            ＋ New session
          </button>
          <p className="session-heading">Sessions</p>
          <div className="session-list">
            {sessions.map((session) => (
              <SessionNavigationItem
                key={session.id}
                session={session}
                active={props.activeSessionID === session.id}
                onOpen={() => props.onOpenSession(session.id)}
                onRename={() => props.onRenameSession(session)}
                onDelete={() => props.onDeleteSession(session)}
              />
            ))}
          </div>
        </div>
      )}
    </article>
  );
}

interface SessionNavigationItemProps {
  session: WorkspaceSession;
  active: boolean;
  onOpen: () => void;
  onRename: () => void;
  onDelete: () => void;
}

function SessionNavigationItem(props: SessionNavigationItemProps) {
  const { session } = props;
  return (
    <div className={`session-item ${session.status}${props.active ? " active" : ""}`}>
      <button className="session-open" type="button" onClick={props.onOpen}>
        <span className="session-status-dot" aria-hidden="true" />
        <span className="session-copy">
          <strong>{session.title}</strong>
          <span>{sessionMeta(session)}</span>
        </span>
      </button>
      <details className="session-menu">
        <summary aria-label="Session actions">•••</summary>
        <div className="context-menu">
          <button type="button" onClick={props.onRename}>
            Rename
          </button>
          <button className="danger" type="button" onClick={props.onDelete}>
            Delete session…
          </button>
        </div>
      </details>
    </div>
  );
}
