import { useEffect, useState } from "react";
import type { AgentDefinition, AgentInput, SkillDefinition } from "../api/contracts";
import type { AgentController } from "./useAgentController";
import type { SkillController } from "./useSkillController";

const emptyAgent: AgentInput = {
  name: "",
  emoji: "✦",
  description: "",
  provider_id: "fireworks",
  model: "",
  max_steps: 12,
  shell_enabled: true,
  instructions: "",
  skill_ids: [],
};

interface AgentEditorProps {
  definition?: AgentDefinition;
  creating: boolean;
  controller: AgentController;
  skillController: SkillController;
  onNavigate: (path: string) => void;
}

export function AgentEditor(props: AgentEditorProps) {
  const [input, setInput] = useState<AgentInput>(emptyAgent);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const readOnly = Boolean(props.definition?.built_in || props.definition?.archived_at);

  useEffect(() => {
    if (!props.definition) {
      setInput(emptyAgent);
      return;
    }
    const { name, emoji, description, provider_id, model, max_steps, shell_enabled, instructions, skill_ids } = props.definition;
    setInput({ name, emoji, description, provider_id, model, max_steps, shell_enabled, instructions, skill_ids });
  }, [props.definition]);

  async function save() {
    setSaving(true);
    setError("");
    try {
      const saved = props.creating
        ? await props.controller.create(input)
        : await props.controller.update(props.definition!.id, input);
      void props.skillController.refresh().catch(() => {});
      props.onNavigate(`/agents/${encodeURIComponent(saved.id)}`);
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setSaving(false);
    }
  }

  if (!props.creating && !props.definition) {
    return <section className="agent-page-scroll"><div className="agent-page-frame"><p className="error">Agent not found.</p></div></section>;
  }

  return (
    <section className="agent-page-scroll">
      <div className="agent-editor-frame">
        <header className="agent-editor-heading">
          <button className="quiet compact" type="button" onClick={() => props.onNavigate("/agents")}>← Agents</button>
          <div>
            <p className="eyebrow">{props.creating ? "New reusable agent" : props.definition?.built_in ? "Built-in agent" : "Agent configuration"}</p>
            <h1>{props.creating ? "Create agent" : props.definition?.name}</h1>
          </div>
          <div className="agent-editor-actions">
            <button className="quiet" type="button" onClick={() => props.onNavigate("/agents")}>Cancel</button>
            {!readOnly && <button className="primary" type="button" disabled={saving || !input.name.trim()} onClick={() => void save()}>{saving ? "Saving…" : "Save agent"}</button>}
          </div>
        </header>
        {readOnly && <div className="agent-readonly-note">{props.definition?.built_in ? "The built-in Ayati agent is protected. Duplicate it to create an editable version." : "Restore this agent before editing it."}</div>}
        {error && <div className="error" role="alert">{error}</div>}
        <div className="agent-editor-sections">
          <EditorSection eyebrow="Identity" title="How this agent appears">
            <div className="agent-identity-fields">
              <label>Emoji<input value={input.emoji} maxLength={16} disabled={readOnly} onChange={(event) => setInput({ ...input, emoji: event.target.value })} /></label>
              <label>Name<input value={input.name} maxLength={60} disabled={readOnly} required onChange={(event) => setInput({ ...input, name: event.target.value })} /></label>
            </div>
            <label>Description<input value={input.description} maxLength={200} disabled={readOnly} placeholder="What is this agent best at?" onChange={(event) => setInput({ ...input, description: event.target.value })} /></label>
          </EditorSection>
          <EditorSection eyebrow="Runtime" title="Model and execution budget">
            <div className="agent-runtime-fields">
              <label>Provider<select value={input.provider_id} disabled={readOnly} onChange={() => {}}><option value="fireworks">Fireworks</option></select></label>
              <label>Model<input value={input.model} disabled={readOnly} placeholder="Use configured default model" onChange={(event) => setInput({ ...input, model: event.target.value })} /></label>
              <label>Step limit<input type="number" min={1} max={20} value={input.max_steps} disabled={readOnly} onChange={(event) => setInput({ ...input, max_steps: Number(event.target.value) })} /></label>
            </div>
          </EditorSection>
          <EditorSection eyebrow="Capabilities" title="Tools available during a run">
            <label className="agent-capability-option">
              <input type="checkbox" checked={input.shell_enabled} disabled={readOnly} onChange={(event) => setInput({ ...input, shell_enabled: event.target.checked })} />
              <span><strong>Workspace shell</strong><small>Uses Ayati’s single shell tool under the workspace’s Explore or Develop authority.</small></span>
            </label>
          </EditorSection>
          <EditorSection eyebrow="Instructions" title="How this agent should approach work">
            <label className="sr-only" htmlFor="agent-instructions">Agent instructions</label>
            <textarea id="agent-instructions" className="agent-instructions" value={input.instructions} disabled={readOnly} maxLength={32768} placeholder="Describe the agent’s responsibilities, priorities and working style…" onChange={(event) => setInput({ ...input, instructions: event.target.value })} />
            <p className="agent-field-note">Workspace authority, credential isolation and publishing rules always take priority.</p>
          </EditorSection>
          <EditorSection eyebrow="Skills" title="Reusable Markdown guidance">
            <AgentSkillPicker
              skills={props.skillController.skills}
              selected={input.skill_ids}
              disabled={readOnly}
              onChange={(skill_ids) => setInput({ ...input, skill_ids })}
              onOpenLibrary={() => props.onNavigate("/agents/skills")}
            />
          </EditorSection>
        </div>
      </div>
    </section>
  );
}

function EditorSection(props: { eyebrow: string; title: string; children: React.ReactNode }) {
  return <section className="agent-editor-section"><div><p className="eyebrow">{props.eyebrow}</p><h2>{props.title}</h2></div><div className="agent-editor-fields">{props.children}</div></section>;
}

function AgentSkillPicker(props: {
  skills: SkillDefinition[];
  selected: string[];
  disabled: boolean;
  onChange: (ids: string[]) => void;
  onOpenLibrary: () => void;
}) {
  const byID = new Map(props.skills.map((skill) => [skill.id, skill]));
  const attached = props.selected.map((id) => byID.get(id)).filter(Boolean) as SkillDefinition[];
  const available = props.skills.filter((skill) => !props.selected.includes(skill.id));

  function move(index: number, offset: number) {
    const next = [...props.selected];
    [next[index], next[index + offset]] = [next[index + offset], next[index]];
    props.onChange(next);
  }

  return <div className="agent-skill-picker">
    {attached.length > 0 ? <div className="attached-skills">{attached.map((skill, index) => <div key={skill.id}>
      <span aria-hidden="true">◇</span><strong>{skill.name}</strong><small>r{skill.revision}</small>
      {!props.disabled && <div>
        <button className="quiet compact" type="button" disabled={index === 0} aria-label={`Move ${skill.name} up`} onClick={() => move(index, -1)}>↑</button>
        <button className="quiet compact" type="button" disabled={index === attached.length - 1} aria-label={`Move ${skill.name} down`} onClick={() => move(index, 1)}>↓</button>
        <button className="quiet compact" type="button" aria-label={`Remove ${skill.name}`} onClick={() => props.onChange(props.selected.filter((id) => id !== skill.id))}>Remove</button>
      </div>}
    </div>)}</div> : <p className="muted skill-picker-empty">No skills attached. Agent instructions will be used on their own.</p>}
    {!props.disabled && available.length > 0 && <label>Add skill<select value="" disabled={props.selected.length >= 12} onChange={(event) => event.target.value && props.onChange([...props.selected, event.target.value])}><option value="">Choose a skill…</option>{available.map((skill) => <option key={skill.id} value={skill.id}>{skill.name}</option>)}</select></label>}
    <div className="skill-picker-footer"><small>{props.selected.length}/12 attached · applied in this order</small><button className="quiet compact" type="button" onClick={props.onOpenLibrary}>Manage skills</button></div>
  </div>;
}
