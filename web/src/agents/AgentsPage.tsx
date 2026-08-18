import { useState } from "react";
import type { AgentDefinition } from "../api/contracts";
import type { AgentController } from "./useAgentController";

interface AgentsPageProps {
  controller: AgentController;
  onNavigate: (path: string) => void;
}

export function AgentsPage({ controller, onNavigate }: AgentsPageProps) {
  const [showArchived, setShowArchived] = useState(false);
  const values = showArchived ? controller.archivedAgents : controller.agents;
  return (
    <section className="agent-page-scroll">
      <div className="agent-page-frame">
        <header className="agent-page-heading">
          <div>
            <p className="eyebrow">Reusable behavior</p>
            <h1>Agents</h1>
            <p className="muted">Create focused agents and use them in any workspace conversation.</p>
          </div>
          <button className="primary" type="button" onClick={() => onNavigate("/agents/new")}>＋ New agent</button>
        </header>
        <div className="agent-list-toolbar">
          <button className={!showArchived ? "active" : ""} type="button" onClick={() => setShowArchived(false)}>Active {controller.agents.length}</button>
          <button className={showArchived ? "active" : ""} type="button" onClick={() => setShowArchived(true)}>Archived {controller.archivedAgents.length}</button>
        </div>
        {controller.error && <div className="error" role="alert">{controller.error}</div>}
        {controller.loading ? (
          <p className="muted">Loading agents…</p>
        ) : values.length ? (
          <div className="agent-card-grid">
            {values.map((definition) => (
              <AgentCard
                key={definition.id}
                definition={definition}
                providerName={controller.providers.find((provider) => provider.id === definition.provider_id)?.name || definition.provider_id}
                archived={showArchived}
                onOpen={() => onNavigate(`/agents/${encodeURIComponent(definition.id)}`)}
                onDefault={() => void controller.makeDefault(definition.id)}
                onDuplicate={async () => {
                  const created = await controller.duplicate(definition.id);
                  onNavigate(`/agents/${encodeURIComponent(created.id)}`);
                }}
                onArchive={() => void controller.archive(definition)}
                onRestore={() => void controller.restore(definition.id)}
              />
            ))}
          </div>
        ) : (
          <div className="agent-empty"><span aria-hidden="true">◇</span><h2>No archived agents</h2><p className="muted">Archived custom agents will appear here.</p></div>
        )}
      </div>
    </section>
  );
}

function AgentCard(props: {
  definition: AgentDefinition;
  providerName: string;
  archived: boolean;
  onOpen: () => void;
  onDefault: () => void;
  onDuplicate: () => void;
  onArchive: () => void;
  onRestore: () => void;
}) {
  const agent = props.definition;
  return (
    <article className={`agent-card${agent.default ? " default" : ""}`}>
      <button className="agent-card-open" type="button" onClick={props.onOpen} aria-label={`Open ${agent.name}`}>
        <span className="agent-card-emoji" aria-hidden="true">{agent.emoji}</span>
        <span className="agent-card-badges">
          {agent.default && <em>Default</em>}
          {agent.built_in && <em>Built-in</em>}
        </span>
        <strong>{agent.name}</strong>
        <span className="agent-card-description">{agent.description || "Custom coding agent"}</span>
        <span className="agent-card-runtime">{props.providerName} · {agent.max_steps} steps</span>
        <span className="agent-card-capability">{agent.shell_enabled ? "Shell enabled" : "Conversation only"}</span>
      </button>
      <div className="agent-card-actions">
        {props.archived ? (
          <button className="quiet compact" type="button" onClick={props.onRestore}>Restore</button>
        ) : (
          <>
            {!agent.default && <button className="quiet compact" type="button" onClick={props.onDefault}>Make default</button>}
            <button className="quiet compact" type="button" onClick={props.onDuplicate}>Duplicate</button>
            {!agent.built_in && !agent.default && <button className="quiet compact danger-text" type="button" onClick={props.onArchive}>Archive</button>}
          </>
        )}
      </div>
    </article>
  );
}
