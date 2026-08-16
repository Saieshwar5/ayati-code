import type { WorkspaceController } from "../app/useWorkspaceController";
import { WorkspaceNavigationItem } from "./WorkspaceNavigationItem";

interface SidebarProps {
  controller: WorkspaceController;
}

export function Sidebar({ controller }: SidebarProps) {
  return (
    <aside className="sidebar" aria-label="Workspace navigation">
      <div className="sidebar-top">
        <a className="brand" href="/" aria-label="Ayati home">
          Ayati
        </a>
        <button className="primary new-workspace" type="button" onClick={controller.showCreate}>
          <span aria-hidden="true">＋</span>
          <span className="nav-label">New workspace</span>
        </button>
      </div>

      <div className="workspace-navigation">
        <p className="nav-heading">Workspaces</p>
        {!controller.workspaces.length && (
          <div className="nav-empty">
            <span>No workspaces yet.</span>
          </div>
        )}
        <div className="workspace-list">
          {controller.workspaces.map((workspace) => (
            <WorkspaceNavigationItem
              key={workspace.id}
              workspace={workspace}
              sessions={controller.sessions[workspace.id] || []}
              expanded={controller.expandedWorkspaceID === workspace.id}
              activeWorkspaceID={controller.activeWorkspace?.id || ""}
              activeSessionID={controller.activeSession?.id || ""}
              onToggle={() => void controller.toggleWorkspace(workspace.id)}
              onOpenSession={(sessionID) => void controller.openWorkspace(workspace.id, sessionID)}
              onCreateSession={() => void controller.createSession(workspace.id)}
              onRenameSession={(session) => void controller.renameSession(workspace.id, session)}
              onDeleteSession={(session) => void controller.deleteSession(workspace.id, session)}
              onAction={(action) => void controller.workspaceAction(workspace.id, action)}
              onDelete={() => void controller.deleteWorkspace(workspace)}
            />
          ))}
        </div>
      </div>

      <div className="sidebar-footer">
        <a
          className="manage-link"
          href="https://github.com/settings/installations"
          target="_blank"
          rel="noreferrer"
        >
          <span aria-hidden="true">↗</span>
          <span className="nav-label">Manage repositories</span>
        </a>
        <div className="account">
          <img className="avatar" src={controller.user.avatar_url} alt="" />
          <span>{controller.user.login}</span>
          <button className="quiet" type="button" onClick={() => void controller.logout()}>
            Sign out
          </button>
        </div>
      </div>
    </aside>
  );
}
