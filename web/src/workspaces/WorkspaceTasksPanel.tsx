import { useState } from "react";

export type WorkspaceTaskStatus = "ready" | "working" | "completed" | "failed";

export interface WorkspaceTask {
  id: string;
  markdown: string;
  status: WorkspaceTaskStatus;
}

interface WorkspaceTasksPanelProps {
  tasks: WorkspaceTask[];
  onCreate: (markdown: string) => void;
  onUpdate: (task: WorkspaceTask) => void;
  onDelete: (taskID: string) => void;
}

const emptyTask = `# Task title

## Goal

Describe what should be accomplished.

## Context

Add important background and decisions.

## Requirements

- Required behavior

## Verification

- How the result should be checked
`;

export function taskMarkdownFromRequest(request: string): string {
  const title = request.trim().split(/[.!?\n]/)[0]?.slice(0, 72) || "New task";
  return emptyTask.replace("Task title", title).replace("Describe what should be accomplished.", request.trim());
}

export function WorkspaceTasksPanel(props: WorkspaceTasksPanelProps) {
  const [editing, setEditing] = useState<WorkspaceTask | "new" | null>(null);
  const [markdown, setMarkdown] = useState(emptyTask);
  const [preview, setPreview] = useState(false);

  function edit(task: WorkspaceTask) {
    setEditing(task);
    setMarkdown(task.markdown);
    setPreview(false);
  }

  function create() {
    setEditing("new");
    setMarkdown(emptyTask);
    setPreview(false);
  }

  function save() {
    if (!markdown.trim()) return;
    if (editing === "new") props.onCreate(markdown);
    else if (editing) props.onUpdate({ ...editing, markdown });
    setEditing(null);
  }

  if (editing) {
    return (
      <aside className="workspace-tasks-panel task-editor" aria-label="Task editor">
        <header className="tasks-panel-heading">
          <div><p className="eyebrow">Markdown task</p><h2>{editing === "new" ? "New task" : taskTitle(editing)}</h2></div>
          <button className="quiet compact" type="button" onClick={() => setEditing(null)}>Close</button>
        </header>
        <div className="task-editor-toggle" role="group" aria-label="Task editor mode">
          <button className={!preview ? "active" : ""} type="button" onClick={() => setPreview(false)}>Edit</button>
          <button className={preview ? "active" : ""} type="button" onClick={() => setPreview(true)}>Preview</button>
        </div>
        {preview ? <pre className="task-markdown-preview">{markdown}</pre> : (
          <textarea aria-label="Task Markdown" value={markdown} onChange={(event) => setMarkdown(event.target.value)} />
        )}
        <footer className="task-editor-actions">
          {editing !== "new" && <button className="quiet compact danger-text" type="button" onClick={() => { props.onDelete(editing.id); setEditing(null); }}>Delete</button>}
          <button className="primary compact" type="button" onClick={save}>Save task</button>
        </footer>
      </aside>
    );
  }

  return (
    <aside className="workspace-tasks-panel" aria-label="Workspace tasks">
      <header className="tasks-panel-heading">
        <div><p className="eyebrow">Workspace</p><h2>Tasks <span>{props.tasks.length}</span></h2></div>
        <button className="tasks-add" type="button" aria-label="Create task" onClick={create}>+</button>
      </header>
      <div className="tasks-panel-list">
        {props.tasks.length ? props.tasks.map((task) => (
          <article className={`workspace-task ${task.status}`} key={task.id}>
            <button type="button" onClick={() => edit(task)}>
              <i aria-hidden="true" />
              <span><strong>{taskTitle(task)}</strong><small>{statusLabel(task.status)}</small></span>
            </button>
          </article>
        )) : (
          <div className="tasks-empty">
            <p>No tasks yet.</p>
            <span>Create one here or use Task mode in the conversation.</span>
            <button className="quiet compact" type="button" onClick={create}>Create task</button>
          </div>
        )}
      </div>
      <footer className="tasks-panel-note">Tasks are UI-only in this milestone. Execution will be connected later.</footer>
    </aside>
  );
}

function taskTitle(task: WorkspaceTask): string {
  return task.markdown.match(/^#\s+(.+)$/m)?.[1]?.trim() || "Untitled task";
}

function statusLabel(status: WorkspaceTaskStatus): string {
  if (status === "working") return "Working";
  if (status === "completed") return "Completed";
  if (status === "failed") return "Failed";
  return "Ready";
}
