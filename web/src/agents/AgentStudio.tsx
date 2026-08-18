import type { AppRoute } from "../app/useAppRoute";
import { AgentStudioNavigation } from "./AgentStudioNavigation";
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
  let content;
  if (props.route.page === "agents") {
    content = <AgentsPage controller={props.controller} onNavigate={props.onNavigate} />;
  } else if (props.route.page === "agent-new") {
    content = <AgentEditor creating controller={props.controller} skillController={props.skills} onNavigate={props.onNavigate} />;
  } else if (props.route.page === "agent-detail") {
    if (props.controller.loading) {
      content = <section className="agent-page-scroll"><div className="agent-page-frame"><p className="muted">Loading agent…</p></div></section>;
    } else {
      const agentID = props.route.agentID;
      const definition = [...props.controller.agents, ...props.controller.archivedAgents]
        .find((candidate) => candidate.id === agentID);
      content = <AgentEditor creating={false} definition={definition} controller={props.controller} skillController={props.skills} onNavigate={props.onNavigate} />;
    }
  } else if (props.route.page === "agent-providers") {
    content = <ProvidersPage controller={props.controller} />;
  } else {
    content = <SkillsPage controller={props.skills} />;
  }

  return (
    <section className="agent-studio-shell">
      <AgentStudioNavigation
        route={props.route}
        agentCount={props.controller.agents.length}
        providerCount={props.controller.providers.length}
        skillCount={props.skills.skills.length}
        onNavigate={props.onNavigate}
      />
      {content}
    </section>
  );
}
