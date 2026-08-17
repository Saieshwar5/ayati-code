import type { AppRoute } from "../app/useAppRoute";

interface AgentStudioSidebarProps {
  route: AppRoute;
  agentCount: number;
  providerCount: number;
  skillCount: number;
  onNavigate: (path: string) => void;
}

export function AgentStudioSidebar(props: AgentStudioSidebarProps) {
  return (
    <aside className="agent-studio-sidebar" aria-label="Agent Studio navigation">
      <header>
        <span className="agent-studio-mark" aria-hidden="true">✦</span>
        <div>
          <p className="eyebrow">Global configuration</p>
          <h2>Agent Studio</h2>
        </div>
      </header>
      <nav>
        <StudioLink
          active={props.route.page === "agents" || props.route.page === "agent-new" || props.route.page === "agent-detail"}
          icon="✦"
          label="Agents"
          count={props.agentCount}
          onClick={() => props.onNavigate("/agents")}
        />
        <StudioLink active={props.route.page === "agent-providers"} icon="◎" label="Providers" count={props.providerCount} onClick={() => props.onNavigate("/agents/providers")} />
        <StudioLink active={props.route.page === "agent-skills"} icon="◇" label="Skills" count={props.skillCount} onClick={() => props.onNavigate("/agents/skills")} />
      </nav>
      <p className="agent-studio-note">Agents are reusable across every workspace. Sessions provide their conversation context.</p>
    </aside>
  );
}

function StudioLink(props: {
  active: boolean;
  icon: string;
  label: string;
  count: number;
  onClick: () => void;
}) {
  return (
    <button className={props.active ? "active" : ""} type="button" aria-current={props.active ? "page" : undefined} onClick={props.onClick}>
      <span aria-hidden="true">{props.icon}</span>
      <strong>{props.label}</strong>
      <small>{props.count}</small>
    </button>
  );
}
