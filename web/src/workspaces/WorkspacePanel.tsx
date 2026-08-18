import { useEffect, useState } from "react";
import type { Changes, PublishInput, Workspace, WorkspaceSession } from "../api/contracts";
import { api } from "../api/client";
import type { WorkspaceController } from "../app/useWorkspaceController";
import { EnvironmentPanel } from "../inspector/EnvironmentPanel";
import { PublishPanel } from "../inspector/PublishPanel";
import { WorkspaceProfilePanel } from "../inspector/WorkspaceProfilePanel";
import { WorkspaceChangesReview } from "./WorkspaceChangesReview";
import { WorkspaceReadiness } from "./WorkspaceReadiness";
import { WorkspaceTasksPanel, type WorkspaceTask } from "./WorkspaceTasksPanel";

export type WorkspacePanelSection = "tasks" | "changes" | "workspace";

interface WorkspacePanelProps {
  workspace: Workspace;
  controller: WorkspaceController;
  section: WorkspacePanelSection;
  expanded: boolean;
  tasks: WorkspaceTask[];
  onSectionChange: (section: WorkspacePanelSection) => void;
  onExpandedChange: (expanded: boolean) => void;
  onClose: () => void;
  onCreateTask: (markdown: string) => void;
  onUpdateTask: (task: WorkspaceTask) => void;
  onDeleteTask: (taskID: string) => void;
  onManageEnvironments: () => void;
  onArchived: () => void;
  onDeleted: () => void;
}

const emptyChanges: Changes = { status: "", diff: "" };

export function WorkspacePanel(props: WorkspacePanelProps) {
  const [changes, setChanges] = useState<Changes>(emptyChanges);
  const [changesError, setChangesError] = useState("");
  const [loadingChanges, setLoadingChanges] = useState(false);
  const [publishOpen, setPublishOpen] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const sessions = props.controller.sessions[props.workspace.id] || [];

  useEffect(() => {
    if (props.section === "changes") void loadChanges();
  }, [props.section, props.workspace.id]);

  async function loadChanges() {
    setLoadingChanges(true);
    setChangesError("");
    try {
      setChanges(await api.changes(props.workspace.id));
    } catch (error) {
      setChanges(emptyChanges);
      setChangesError((error as Error).message);
    } finally {
      setLoadingChanges(false);
    }
  }

  async function publish(input: PublishInput) {
    setPublishing(true);
    try {
      const updated = await api.publish(props.workspace.id, input);
      props.controller.updateWorkspace(updated);
      await loadChanges();
      setPublishOpen(false);
      return true;
    } finally {
      setPublishing(false);
    }
  }

  async function archive() {
    if (await props.controller.archiveWorkspace(props.workspace)) props.onArchived();
  }

  async function remove() {
    if (await props.controller.deleteWorkspace(props.workspace)) props.onDeleted();
  }

  return (
    <aside className={`workspace-side-panel${props.expanded ? " expanded" : ""}`} aria-label="Workspace panel">
      <header className="workspace-panel-heading">
        <div><p className="eyebrow">Workspace</p><h2>{panelTitle(props.section)}</h2></div>
        <div>
          <button type="button" aria-label={props.expanded ? "Dock workspace panel" : "Expand workspace panel"} onClick={() => props.onExpandedChange(!props.expanded)}>{props.expanded ? "↘" : "↖"}</button>
          <button type="button" aria-label="Close workspace panel" onClick={props.onClose}>×</button>
        </div>
      </header>
      <nav className="workspace-panel-tabs" aria-label="Workspace panel sections">
        <PanelTab name="tasks" label="Tasks" count={props.tasks.length} selected={props.section} onSelect={props.onSectionChange} />
        <PanelTab name="changes" label="Changes" selected={props.section} onSelect={props.onSectionChange} />
        <PanelTab name="workspace" label="Workspace" selected={props.section} onSelect={props.onSectionChange} />
      </nav>
      <div className="workspace-panel-content">
        {props.section === "tasks" ? (
          <WorkspaceTasksPanel tasks={props.tasks} onCreate={props.onCreateTask} onUpdate={props.onUpdateTask} onDelete={props.onDeleteTask} />
        ) : props.section === "changes" ? (
          <WorkspaceChangesReview changes={changes} loading={loadingChanges} error={changesError} compact={!props.expanded} onFileOpen={() => props.onExpandedChange(true)} onRefresh={() => void loadChanges()} onPublish={() => setPublishOpen(true)} />
        ) : (
          <WorkspaceDetails
            workspace={props.workspace}
            controller={props.controller}
            sessions={sessions}
            expanded={props.expanded}
            onManageEnvironments={props.onManageEnvironments}
            onArchive={archive}
            onDelete={remove}
          />
        )}
      </div>
      {publishOpen && (
        <div className="workspace-dialog-backdrop" role="presentation" onMouseDown={() => setPublishOpen(false)}>
          <section className="workspace-dialog" role="dialog" aria-modal="true" aria-label="Publish workspace changes" onMouseDown={(event) => event.stopPropagation()}>
            <button className="dialog-close" type="button" aria-label="Close publish dialog" onClick={() => setPublishOpen(false)}>×</button>
            <PublishPanel workspace={props.workspace} publishing={publishing} onPublish={publish} />
          </section>
        </div>
      )}
    </aside>
  );
}

function PanelTab(props: { name: WorkspacePanelSection; label: string; count?: number; selected: WorkspacePanelSection; onSelect: (section: WorkspacePanelSection) => void }) {
  return <button className={props.selected === props.name ? "active" : ""} type="button" onClick={() => props.onSelect(props.name)}>{props.label}{props.count !== undefined && <span>{props.count}</span>}</button>;
}

function WorkspaceDetails(props: {
  workspace: Workspace;
  controller: WorkspaceController;
  sessions: WorkspaceSession[];
  expanded: boolean;
  onManageEnvironments: () => void;
  onArchive: () => Promise<void>;
  onDelete: () => Promise<void>;
}) {
  const working = props.sessions.some((session) => session.status === "working");
  return (
    <div className={`workspace-panel-details${props.expanded ? " expanded" : ""}`}>
      {props.workspace.status !== "ready" && (
        <WorkspaceReadiness
          workspace={props.workspace}
          embedded
          onConfigure={(root) => props.controller.configureProjectRoot(props.workspace.id, root)}
          onRetry={() => props.controller.workspaceAction(props.workspace.id, "initialize")}
          onResume={() => props.controller.workspaceAction(props.workspace.id, "resume")}
          onDelete={props.onDelete}
        />
      )}
      <details open>
        <summary>Project details</summary>
        <WorkspaceProfilePanel workspace={props.workspace} agentWorking={working} onAuthorityChange={(input) => props.controller.changeWorkspaceAuthority(props.workspace.id, input)} />
      </details>
      <details>
        <summary>Environment variables</summary>
        <EnvironmentPanel workspace={props.workspace} sessions={props.sessions} />
      </details>
      <details>
        <summary>Lifecycle</summary>
        <div className="workspace-panel-lifecycle">
          <button className="quiet compact" type="button" onClick={props.onManageEnvironments}>Manage compute</button>
          <button className="quiet compact" type="button" onClick={() => void props.onArchive()}>Archive workspace…</button>
          <button className="quiet compact danger-text" type="button" onClick={() => void props.onDelete()}>Delete workspace…</button>
        </div>
      </details>
    </div>
  );
}

function panelTitle(section: WorkspacePanelSection): string {
  if (section === "tasks") return "Tasks";
  if (section === "changes") return "Changes";
  return "Details";
}
