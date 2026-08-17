import { useRef, useState } from "react";
import type { ChangeEvent } from "react";
import type { SkillDefinition, SkillInput } from "../api/contracts";
import type { SkillController } from "./useSkillController";

const emptySkill: SkillInput = { name: "", description: "", markdown: "" };

export function SkillsPage({ controller }: { controller: SkillController }) {
  const [showArchived, setShowArchived] = useState(false);
  const [editingID, setEditingID] = useState<string | null>(null);
  const [draft, setDraft] = useState<SkillInput | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const values = showArchived ? controller.archivedSkills : controller.skills;

  function edit(skill: SkillDefinition) {
    setEditingID(skill.id);
    setDraft({ name: skill.name, description: skill.description, markdown: skill.markdown });
  }

  async function importMarkdown(event: ChangeEvent<HTMLInputElement>) {
    const file = event.target.files?.[0];
    event.target.value = "";
    if (!file) return;
    const markdown = await file.text();
    setEditingID(null);
    setDraft({ name: file.name.replace(/\.md$/i, ""), description: "Imported Markdown skill", markdown });
  }

  return (
    <section className="agent-page-scroll">
      <div className="agent-page-frame">
        <header className="agent-page-heading">
          <div><p className="eyebrow">Reusable instructions</p><h1>Skills</h1><p className="muted">Compose focused Markdown guidance into any custom agent.</p></div>
          <div className="skill-heading-actions">
            <input ref={fileInput} className="sr-only" type="file" accept=".md,text/markdown,text/plain" aria-label="Import Markdown skill" onChange={(event) => void importMarkdown(event)} />
            <button className="quiet" type="button" onClick={() => fileInput.current?.click()}>Import .md</button>
            <button className="primary" type="button" onClick={() => { setEditingID(null); setDraft(emptySkill); }}>＋ New skill</button>
          </div>
        </header>
        {controller.error && <div className="error" role="alert">{controller.error}</div>}
        <div className="agent-list-toolbar">
          <button className={!showArchived ? "active" : ""} type="button" onClick={() => setShowArchived(false)}>Active {controller.skills.length}</button>
          <button className={showArchived ? "active" : ""} type="button" onClick={() => setShowArchived(true)}>Archived {controller.archivedSkills.length}</button>
        </div>
        <div className={`skills-layout${draft ? " editing" : ""}`}>
          <div className="skill-list">
            {controller.loading ? <p className="muted">Loading skills…</p> : values.map((skill) => (
              <SkillCard
                key={skill.id}
                skill={skill}
                archived={showArchived}
                onEdit={() => edit(skill)}
                onArchive={() => void controller.archive(skill)}
                onRestore={() => void controller.restore(skill.id)}
                onExport={() => exportSkill(skill)}
              />
            ))}
            {!controller.loading && values.length === 0 && <div className="agent-empty"><span aria-hidden="true">◇</span><h2>{showArchived ? "No archived skills" : "No skills yet"}</h2><p className="muted">{showArchived ? "Archived skills will appear here." : "Create or import reusable Markdown guidance."}</p></div>}
          </div>
          {draft && <SkillEditor
            input={draft}
            existing={editingID !== null}
            onInput={setDraft}
            onCancel={() => setDraft(null)}
            onSave={async () => {
              if (editingID) await controller.update(editingID, draft);
              else await controller.create(draft);
              setDraft(null);
            }}
          />}
        </div>
      </div>
    </section>
  );
}

function SkillCard(props: {
  skill: SkillDefinition;
  archived: boolean;
  onEdit: () => void;
  onArchive: () => void;
  onRestore: () => void;
  onExport: () => void;
}) {
  const skill = props.skill;
  return <article className="skill-card">
    <button className="skill-card-open" type="button" disabled={props.archived} onClick={props.onEdit} aria-label={`Edit ${skill.name}`}>
      <span aria-hidden="true">◇</span><div><strong>{skill.name}</strong><p>{skill.description || "Markdown guidance"}</p><small>Revision {skill.revision} · {skill.attached_agents} agent{skill.attached_agents === 1 ? "" : "s"}</small></div>
    </button>
    <div className="skill-card-actions">
      <button className="quiet compact" type="button" onClick={props.onExport}>Export .md</button>
      {props.archived ? <button className="quiet compact" type="button" onClick={props.onRestore}>Restore</button> : <>
        <button className="quiet compact" type="button" onClick={props.onEdit}>Edit</button>
        <button className="quiet compact danger-text" type="button" disabled={skill.attached_agents > 0} title={skill.attached_agents > 0 ? "Detach this skill from its agents first" : undefined} onClick={props.onArchive}>Archive</button>
      </>}
    </div>
  </article>;
}

function exportSkill(skill: SkillDefinition) {
  const blob = new Blob([skill.markdown + "\n"], { type: "text/markdown;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `${skill.name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "") || "skill"}.md`;
  link.click();
  URL.revokeObjectURL(url);
}

function SkillEditor(props: {
  input: SkillInput;
  existing: boolean;
  onInput: (input: SkillInput) => void;
  onCancel: () => void;
  onSave: () => Promise<void>;
}) {
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  async function save() {
    setSaving(true);
    setError("");
    try { await props.onSave(); } catch (reason) { setError((reason as Error).message); } finally { setSaving(false); }
  }
  return <aside className="skill-editor" aria-label={props.existing ? "Edit skill" : "Create skill"}>
    <header><div><p className="eyebrow">{props.existing ? "Edit skill" : "New skill"}</p><h2>{props.input.name || "Untitled skill"}</h2></div><button className="quiet compact" type="button" onClick={props.onCancel}>Close</button></header>
    {error && <div className="error" role="alert">{error}</div>}
    <label>Name<input value={props.input.name} maxLength={80} onChange={(event) => props.onInput({ ...props.input, name: event.target.value })} /></label>
    <label>Description<input value={props.input.description} maxLength={240} onChange={(event) => props.onInput({ ...props.input, description: event.target.value })} /></label>
    <label>Markdown<textarea value={props.input.markdown} maxLength={32768} placeholder="# Skill name&#10;&#10;Describe the reusable guidance…" onChange={(event) => props.onInput({ ...props.input, markdown: event.target.value })} /></label>
    <p className="agent-field-note">Markdown is prompt guidance only. It cannot add tools or override workspace rules.</p>
    <button className="primary" type="button" disabled={saving || !props.input.name.trim() || !props.input.markdown.trim()} onClick={() => void save()}>{saving ? "Saving…" : "Save skill"}</button>
  </aside>;
}
