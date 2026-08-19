import { useState } from "react";
import type { AgentDefinition } from "../api/contracts";
import { Icon } from "../ui/Icon";
import type { AgentController } from "./useAgentController";

interface AgentsPageProps {
  controller: AgentController;
  onNavigate: (path: string) => void;
}

export function AgentsPage({ controller, onNavigate }: AgentsPageProps) {
  const [showArchived, setShowArchived] = useState(false);
  const [query, setQuery] = useState("");
  const source = showArchived ? controller.archivedAgents : controller.agents;
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const values = [...source]
    .filter((definition) => {
      const provider = controller.providers.find((candidate) => candidate.id === definition.provider_id)?.name || definition.provider_id;
      return !normalizedQuery || [definition.name, definition.description, definition.model, provider]
        .some((value) => value.toLocaleLowerCase().includes(normalizedQuery));
    })
    .sort((left, right) => Number(right.default) - Number(left.default) || left.name.localeCompare(right.name));
  return (
    <section className="agent-page-scroll">
      <div className="agent-page-frame">
        <header className="agent-page-heading">
          <div>
            <h1>Agents</h1>
            <p className="muted">Create focused agents and use them in any workspace conversation.</p>
          </div>
          <button className="primary agent-new-button" type="button" onClick={() => onNavigate("/agents/new")}><Icon name="plus" />New agent</button>
        </header>
        <div className="agent-list-controls">
          <label className="agent-search">
            <span className="sr-only">Search agents</span>
            <input type="search" placeholder="Search agents" value={query} onChange={(event) => setQuery(event.target.value)} />
          </label>
          <div className="agent-status-filter" role="group" aria-label="Agent status">
            <button className={!showArchived ? "active" : ""} type="button" aria-pressed={!showArchived} onClick={() => setShowArchived(false)}>Active <span>{controller.agents.length}</span></button>
            <button className={showArchived ? "active" : ""} type="button" aria-pressed={showArchived} onClick={() => setShowArchived(true)}>Archived <span>{controller.archivedAgents.length}</span></button>
          </div>
        </div>
        {controller.error && <div className="error" role="alert">{controller.error}</div>}
        {controller.loading ? (
          <p className="muted">Loading agents…</p>
        ) : values.length ? (
          <div className="agent-list-table">
            <div className="agent-list-header" aria-hidden="true"><span>Agent</span><span>Runtime</span><span>Skills</span><span>Status</span><span /></div>
            {values.map((definition) => (
              <AgentRow
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
          <div className="agent-empty"><Icon name="agents" /><h2>{normalizedQuery ? "No matching agents" : showArchived ? "No archived agents" : "No agents yet"}</h2><p className="muted">{normalizedQuery ? "Try another name, description, model, or provider." : showArchived ? "Archived custom agents will appear here." : "Create an agent to define reusable behavior."}</p></div>
        )}
      </div>
    </section>
  );
}

function AgentRow(props: {
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
  const model = agent.model || "Default model";
  return (
    <article className={`agent-row${agent.default ? " default" : ""}`}>
      <button className="agent-row-open" type="button" onClick={props.onOpen} aria-label={`Open ${agent.name}`}>
        <span className="agent-row-identity">
          <span className="agent-row-emoji" aria-hidden="true">{agent.emoji}</span>
          <span className="agent-row-copy"><strong>{agent.name}</strong><small>{agent.description || "Custom coding agent"}</small></span>
        </span>
        <span className="agent-row-runtime"><strong>{props.providerName}</strong><small>{model} · {agent.max_steps} steps</small></span>
        <span className="agent-row-skills">{agent.skill_ids.length ? `${agent.skill_ids.length} skill${agent.skill_ids.length === 1 ? "" : "s"}` : "None"}</span>
        <span className="agent-row-badges">
          {props.archived && <em>Archived</em>}
          {agent.default && <em>Default</em>}
          {agent.built_in && <em>Built-in</em>}
          {!agent.shell_enabled && <em>Conversation only</em>}
        </span>
      </button>
      <details className="agent-row-menu">
        <summary aria-label={`Actions for ${agent.name}`}><Icon name="more" /></summary>
        <div className="context-menu">
          {props.archived ? (
            <button type="button" onClick={props.onRestore}>Restore</button>
          ) : (
            <>
              {!agent.default && <button type="button" onClick={props.onDefault}>Make default</button>}
              <button type="button" onClick={props.onDuplicate}>Duplicate</button>
              {!agent.built_in && !agent.default && <button className="danger-text" type="button" onClick={props.onArchive}>Archive</button>}
            </>
          )}
        </div>
      </details>
    </article>
  );
}
