import type { AppRoute } from "../app/useAppRoute";

interface AgentStudioNavigationProps {
  route: AppRoute;
  agentCount: number;
  providerCount: number;
  skillCount: number;
  onNavigate: (path: string) => void;
}

export function AgentStudioNavigation(props: AgentStudioNavigationProps) {
  return (
    <nav className="agent-studio-navigation" aria-label="Agent settings">
      <StudioTab
        active={props.route.page === "agents" || props.route.page === "agent-new" || props.route.page === "agent-detail"}
        label="Agents"
        count={props.agentCount}
        onClick={() => props.onNavigate("/agents")}
      />
      <StudioTab active={props.route.page === "agent-providers"} label="Providers" count={props.providerCount} onClick={() => props.onNavigate("/agents/providers")} />
      <StudioTab active={props.route.page === "agent-skills"} label="Skills" count={props.skillCount} onClick={() => props.onNavigate("/agents/skills")} />
    </nav>
  );
}

function StudioTab(props: { active: boolean; label: string; count: number; onClick: () => void }) {
  return (
    <button type="button" aria-current={props.active ? "page" : undefined} onClick={props.onClick}>
      <span>{props.label}</span>
      <small>{props.count}</small>
    </button>
  );
}
