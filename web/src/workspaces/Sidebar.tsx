import type { AppRoute } from "../app/useAppRoute";
import { isAgentRoute, workspacePath } from "../app/useAppRoute";
import type { WorkspaceController } from "../app/useWorkspaceController";
import { Icon, type IconName } from "../ui/Icon";
import { WorkspaceNavigationItem } from "./WorkspaceNavigationItem";

interface SidebarProps {
  controller: WorkspaceController;
  route: AppRoute;
  collapsed: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
  onNavigate: (path: string) => void;
}

export function Sidebar({ controller, route, collapsed, onCollapsedChange, onNavigate }: SidebarProps) {
  const activeWorkspaceID = "workspaceID" in route ? route.workspaceID : "";
  return (
    <aside className="sidebar" aria-label="Main navigation">
      <div className="sidebar-top">
        <div className="brand-row">
          <button className="brand" type="button" onClick={() => onNavigate("/workspaces")}>
            <span className="brand-name">perpetual</span>
          </button>
          <button
            aria-expanded={!collapsed}
            aria-label={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            className="sidebar-toggle"
            title={collapsed ? "Expand sidebar" : "Collapse sidebar"}
            type="button"
            onClick={() => onCollapsedChange(!collapsed)}
          >
            <Icon name={collapsed ? "panelOpen" : "panelClose"} />
          </button>
        </div>
      </div>

      <nav className="product-navigation" aria-label="Product">
        <div className="product-nav-row">
          <NavButton label="Workspaces" icon="workspaces" active={route.page === "workspaces" || route.page === "workspace" || route.page === "create-workspace"} onClick={() => onNavigate("/workspaces")} />
          <button
            aria-label="Create workspace from navigation"
            className="product-nav-create"
            title="Create workspace"
            type="button"
            onClick={() => onNavigate("/workspaces/new")}
          >
            <Icon name="plus" />
          </button>
        </div>
        <NavButton label="Agents" icon="agents" active={isAgentRoute(route)} onClick={() => onNavigate("/agents")} />
        <NavButton label="Environments" icon="environments" active={route.page === "environments"} onClick={() => onNavigate("/environments")} />
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
          <Icon name="archive" />
          <span>Archived</span>
          {controller.archivedWorkspaces.length > 0 && <strong>{controller.archivedWorkspaces.length}</strong>}
        </button>
      </div>

      <div className="sidebar-footer">
        <a className="manage-link" href="https://github.com/settings/installations" target="_blank" rel="noreferrer">
          <Icon name="external" />
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

function NavButton(props: { label: string; icon: IconName; active: boolean; onClick: () => void }) {
  return (
    <button className={`product-nav-item${props.active ? " active" : ""}`} type="button" aria-current={props.active ? "page" : undefined} onClick={props.onClick}>
      <Icon name={props.icon} />
      <strong>{props.label}</strong>
    </button>
  );
}
