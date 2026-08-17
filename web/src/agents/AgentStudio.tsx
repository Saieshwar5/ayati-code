import type { AppRoute } from "../app/useAppRoute";
import { AgentsPage } from "./AgentsPage";
import { AgentEditor } from "./AgentEditor";
import type { AgentController } from "./useAgentController";

interface AgentStudioProps {
  route: AppRoute;
  controller: AgentController;
  onNavigate: (path: string) => void;
}

export function AgentStudio(props: AgentStudioProps) {
  if (props.route.page === "agents") {
    return <AgentsPage controller={props.controller} onNavigate={props.onNavigate} />;
  }
  if (props.route.page === "agent-new") {
    return <AgentEditor creating controller={props.controller} onNavigate={props.onNavigate} />;
  }
  if (props.route.page === "agent-detail") {
    if (props.controller.loading) {
      return <section className="agent-page-scroll"><div className="agent-page-frame"><p className="muted">Loading agent…</p></div></section>;
    }
    const agentID = props.route.agentID;
    const definition = [...props.controller.agents, ...props.controller.archivedAgents]
      .find((candidate) => candidate.id === agentID);
    return <AgentEditor creating={false} definition={definition} controller={props.controller} onNavigate={props.onNavigate} />;
  }
  if (props.route.page === "agent-providers") {
    return <UpcomingPage eyebrow="Model access" title="Providers" glyph="◎" description="Fireworks is Ayati’s configured provider. Write-only provider management will be implemented in its own focused branch." action="1 provider available" />;
  }
  return <UpcomingPage eyebrow="Reusable instructions" title="Skills" glyph="◇" description="Create, import and attach managed Markdown skills from this global library in the next focused branch." action="Markdown skills planned" />;
}

function UpcomingPage(props: {
  eyebrow: string;
  title: string;
  glyph: string;
  description: string;
  action: string;
}) {
  return (
    <section className="agent-page-scroll">
      <div className="agent-page-frame narrow">
        <div className="agent-upcoming-page">
          <span aria-hidden="true">{props.glyph}</span>
          <p className="eyebrow">{props.eyebrow}</p>
          <h1>{props.title}</h1>
          <p className="muted">{props.description}</p>
          <em>{props.action}</em>
        </div>
      </div>
    </section>
  );
}
