import type { AppRoute } from "../app/useAppRoute";
import { AgentsPage } from "./AgentsPage";
import { AgentEditor } from "./AgentEditor";
import { ProvidersPage } from "./ProvidersPage";
import { SkillsPage } from "./SkillsPage";
import type { AgentController } from "./useAgentController";
import type { SkillController } from "./useSkillController";

interface AgentStudioProps {
  route: AppRoute;
  controller: AgentController;
  skills: SkillController;
  onNavigate: (path: string) => void;
}

export function AgentStudio(props: AgentStudioProps) {
  if (props.route.page === "agents") {
    return <AgentsPage controller={props.controller} onNavigate={props.onNavigate} />;
  }
  if (props.route.page === "agent-new") {
    return <AgentEditor creating controller={props.controller} skillController={props.skills} onNavigate={props.onNavigate} />;
  }
  if (props.route.page === "agent-detail") {
    if (props.controller.loading) {
      return <section className="agent-page-scroll"><div className="agent-page-frame"><p className="muted">Loading agent…</p></div></section>;
    }
    const agentID = props.route.agentID;
    const definition = [...props.controller.agents, ...props.controller.archivedAgents]
      .find((candidate) => candidate.id === agentID);
    return <AgentEditor creating={false} definition={definition} controller={props.controller} skillController={props.skills} onNavigate={props.onNavigate} />;
  }
  if (props.route.page === "agent-providers") {
    return <ProvidersPage controller={props.controller} />;
  }
  return <SkillsPage controller={props.skills} />;
}
