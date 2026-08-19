import type { AppRoute } from "../app/useAppRoute";
import { isAgentRoute, workspacePath } from "../app/useAppRoute";
import type { WorkspaceController } from "../app/useWorkspaceController";
import { WorkspaceNavigationItem } from "./WorkspaceNavigationItem";

interface SidebarProps {
  controller: WorkspaceController;
  route: AppRoute;
  onNavigate: (path: string) => void;
}

export function Sidebar({ controller, route, onNavigate }: SidebarProps) {
  const activeWorkspaceID = "workspaceID" in route ? route.workspaceID : "";
  return (
    <aside className="sidebar" aria-label="Main navigation">
      <div className="sidebar-top">
        <button className="brand" type="button" onClick={() => onNavigate("/workspaces")}>
          <span className="brand-mark" aria-hidden="true">A</span>
          <span>Perpetual</span>
        </button>
        <button className="primary new-workspace" type="button" onClick={() => onNavigate("/workspaces/new")}>
          <span aria-hidden="true">＋</span>
          <span>New workspace</span>
        </button>
      </div>

      <nav className="product-navigation" aria-label="Product">
        <NavButton label="Workspaces" icon="◇" active={route.page === "workspaces" || route.page === "workspace" || route.page === "session" || route.page === "create-workspace"} onClick={() => onNavigate("/workspaces")} />
        <NavButton label="Agents" icon="✦" active={isAgentRoute(route)} onClick={() => onNavigate("/agents")} />
        <NavButton label="Environments" icon="⌁" active={route.page === "environments"} onClick={() => onNavigate("/environments")} />
      </nav>

      <div className="workspace-navigation">
        <div className="nav-section-heading">
          <p className="nav-heading">Recent workspaces</p>
          <span>{controller.workspaces.length}</span>
        </div>
        {!controller.workspaces.length && <p className="nav-empty">No active workspaces.</p>}
        <div className="workspace-list">
          {controller.workspaces.slice(0, 8).map((workspace) => (
            <WorkspaceNavigationItem
              key={workspace.id}
              workspace={workspace}
              active={activeWorkspaceID === workspace.id}
              onOpen={() => onNavigate(workspacePath(workspace.id))}
            />
          ))}
        </div>
        <button
          className={`archive-link${route.page === "archived" ? " active" : ""}`}
          type="button"
          onClick={() => onNavigate("/workspaces/archived")}
        >
          <span aria-hidden="true">▱</span>
          <span>Archived</span>
          {controller.archivedWorkspaces.length > 0 && <strong>{controller.archivedWorkspaces.length}</strong>}
        </button>
      </div>

      <div className="sidebar-footer">
        <a className="manage-link" href="https://github.com/settings/installations" target="_blank" rel="noreferrer">
          <span aria-hidden="true">↗</span>
          <span>Manage repositories</span>
        </a>
        <div className="account">
          <img className="avatar" src={controller.user.avatar_url} alt="" />
          <span>{controller.user.login}</span>
          <button className="quiet" type="button" onClick={() => void controller.logout()}>Sign out</button>
        </div>
      </div>
    </aside>
  );
}

function NavButton(props: { label: string; icon: string; active: boolean; onClick: () => void }) {
  return (
    <button className={`product-nav-item${props.active ? " active" : ""}`} type="button" aria-current={props.active ? "page" : undefined} onClick={props.onClick}>
      <span aria-hidden="true">{props.icon}</span>
      <strong>{props.label}</strong>
    </button>
  );
}
