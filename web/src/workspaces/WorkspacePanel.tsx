import { useEffect, useState } from "react";
import type { Changes, PublishInput, Workspace, WorkspaceSession } from "../api/contracts";
import { api } from "../api/client";
import { repositoryName, statusLabel } from "../app/format";
import type { WorkspaceController } from "../app/useWorkspaceController";
import { AuthorityControl } from "../inspector/AuthorityControl";
import { EnvironmentPanel } from "../inspector/EnvironmentPanel";
import { PublishPanel } from "../inspector/PublishPanel";
import { WorkspaceProfilePanel } from "../inspector/WorkspaceProfilePanel";
import { Icon } from "../ui/Icon";
import { WorkspaceActivityRail } from "./WorkspaceActivityRail";
import { countChangedFiles, WorkspaceChangesReview } from "./WorkspaceChangesReview";
import { WorkspaceReadiness } from "./WorkspaceReadiness";
import { WorkspaceTasksPanel, type WorkspaceTask } from "./WorkspaceTasksPanel";

export type WorkspacePanelSection = "tasks" | "changes" | "workspace";

interface WorkspacePanelProps {
  workspace: Workspace;
  controller: WorkspaceController;
  open: boolean;
  section: WorkspacePanelSection;
  expanded: boolean;
  tasks: WorkspaceTask[];
  onOpenChange: (open: boolean) => void;
  onSectionChange: (section: WorkspacePanelSection) => void;
  onExpandedChange: (expanded: boolean) => void;
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
  const changeCount = countChangedFiles(changes);

  useEffect(() => {
    void loadChanges();
  }, [props.workspace.id]);

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

  function selectSection(section: WorkspacePanelSection) {
    if (props.open && props.section === section) {
      props.onOpenChange(false);
      props.onExpandedChange(false);
      return;
    }
    props.onSectionChange(section);
    props.onOpenChange(true);
  }

  async function archive() {
    if (await props.controller.archiveWorkspace(props.workspace)) props.onArchived();
  }

  async function remove() {
    if (await props.controller.deleteWorkspace(props.workspace)) props.onDeleted();
  }

  return (
    <>
      {props.open && (
        <aside className={`workspace-side-panel${props.expanded ? " expanded" : ""}`} aria-label="Workspace panel">
          <header className="workspace-panel-heading">
            <div><h2>{panelTitle(props.section)}</h2><p>{panelSubtitle(props.section, props.tasks.length, changeCount, props.workspace)}</p></div>
            <div>
              <button type="button" aria-label={props.expanded ? "Dock workspace panel" : "Focus workspace panel"} title={props.expanded ? "Dock panel" : "Focus panel"} onClick={() => props.onExpandedChange(!props.expanded)}><Icon name={props.expanded ? "dock" : "focus"} /></button>
              <button type="button" aria-label="Collapse workspace panel" title="Collapse panel" onClick={() => { props.onOpenChange(false); props.onExpandedChange(false); }}><Icon name="panelClose" /></button>
            </div>
          </header>
          <div className="workspace-panel-content">
            {props.section === "tasks" ? (
              <WorkspaceTasksPanel embedded tasks={props.tasks} onCreate={props.onCreateTask} onUpdate={props.onUpdateTask} onDelete={props.onDeleteTask} />
            ) : props.section === "changes" ? (
              <WorkspaceChangesReview embedded changes={changes} loading={loadingChanges} error={changesError} compact={!props.expanded} onFileOpen={() => props.onExpandedChange(true)} onRefresh={() => void loadChanges()} onPublish={() => setPublishOpen(true)} />
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
        </aside>
      )}
      <WorkspaceActivityRail
        open={props.open}
        selected={props.section}
        taskCount={props.tasks.length}
        changeCount={changeCount}
        workspaceStatus={props.workspace.status}
        onSelect={selectSection}
      />
      {publishOpen && (
        <div className="workspace-dialog-backdrop" role="presentation" onMouseDown={() => setPublishOpen(false)}>
          <section className="workspace-dialog" role="dialog" aria-modal="true" aria-label="Publish workspace changes" onMouseDown={(event) => event.stopPropagation()}>
            <button className="dialog-close" type="button" aria-label="Close publish dialog" onClick={() => setPublishOpen(false)}>×</button>
            <PublishPanel workspace={props.workspace} publishing={publishing} onPublish={publish} />
          </section>
        </div>
      )}
    </>
  );
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
        <WorkspaceReadiness workspace={props.workspace} embedded onConfigure={(root) => props.controller.configureProjectRoot(props.workspace.id, root)} onRetry={() => props.controller.workspaceAction(props.workspace.id, "initialize")} onResume={() => props.controller.workspaceAction(props.workspace.id, "resume")} onDelete={props.onDelete} />
      )}
      <section className="workspace-utility-overview">
        <div><span>Status</span><strong>{statusLabel(props.workspace.status)}</strong></div>
        <div><span>Repository</span><strong>{props.workspace.repository}</strong></div>
        <div><span>Branch</span><code>{props.workspace.branch}</code></div>
        <div><span>Base</span><code>{props.workspace.base_branch}</code></div>
      </section>
      <details>
        <summary><span>Access</span><small>{props.workspace.authority === "explore" ? "Explore" : "Develop"}</small></summary>
        <div className="workspace-utility-detail"><AuthorityControl workspace={props.workspace} agentWorking={working} onChange={(input) => props.controller.changeWorkspaceAuthority(props.workspace.id, input)} /></div>
      </details>
      <details>
        <summary><span>Project profile</span><small>{repositoryName(props.workspace.repository)}</small></summary>
        <WorkspaceProfilePanel workspace={props.workspace} agentWorking={working} showAuthority={false} />
      </details>
      <details>
        <summary><span>Environment variables</span><small>Configure</small></summary>
        <EnvironmentPanel workspace={props.workspace} sessions={props.sessions} />
      </details>
      <details>
        <summary><span>Lifecycle</span><small>Manage</small></summary>
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
  return "Workspace";
}

function panelSubtitle(section: WorkspacePanelSection, taskCount: number, changeCount: number, workspace: Workspace): string {
  if (section === "tasks") return `${taskCount} ${taskCount === 1 ? "task" : "tasks"}`;
  if (section === "changes") return changeCount ? `${changeCount} changed ${changeCount === 1 ? "file" : "files"}` : "Working tree clean";
  return `${repositoryName(workspace.repository)} · ${statusLabel(workspace.status)}`;
}
