import { useEffect, useState } from "react";
import type { Changes, PublishInput, Workspace, WorkspaceSession } from "../api/contracts";
import { api } from "../api/client";
import { repositoryName, statusLabel } from "../app/format";
import type { WorkspaceController } from "../app/useWorkspaceController";
import { EnvironmentPanel } from "../inspector/EnvironmentPanel";
import { PublishPanel } from "../inspector/PublishPanel";
import { WorkspaceProfilePanel } from "../inspector/WorkspaceProfilePanel";
import { WorkspaceCapacity } from "./WorkspaceCapacity";
import { countChangedFiles, WorkspaceChangesReview } from "./WorkspaceChangesReview";
import { WorkspaceReadiness } from "./WorkspaceReadiness";
import { WorkspaceTasksPanel, type WorkspaceTask } from "./WorkspaceTasksPanel";

interface WorkspaceOverviewProps {
  workspace: Workspace;
  sessions: WorkspaceSession[];
  controller: WorkspaceController;
  tasks: WorkspaceTask[];
  onBack: () => void;
  onOpenConversation: () => void;
  onManageEnvironments: () => void;
  onCreateTask: (markdown: string) => void;
  onUpdateTask: (task: WorkspaceTask) => void;
  onDeleteTask: (taskID: string) => void;
  onArchived: () => void;
  onDeleted: () => void;
}

const emptyChanges: Changes = { status: "", diff: "" };

export function WorkspaceOverview(props: WorkspaceOverviewProps) {
  const { workspace } = props;
  const [changes, setChanges] = useState<Changes>(emptyChanges);
  const [changesError, setChangesError] = useState("");
  const [loadingChanges, setLoadingChanges] = useState(false);
  const [reviewOpen, setReviewOpen] = useState(false);
  const [publishOpen, setPublishOpen] = useState(false);
  const [publishing, setPublishing] = useState(false);
  const changeCount = countChangedFiles(changes);
  const deleting = props.controller.deletingWorkspaceIDs.has(workspace.id) || workspace.status === "deleting";
  const conversationReady = workspace.status === "ready";

  useEffect(() => {
    setChanges(emptyChanges);
    setChangesError("");
    setReviewOpen(false);
    setPublishOpen(false);
    if (workspace.status === "ready") void loadChanges();
  }, [workspace.id, workspace.status]);

  async function loadChanges() {
    setLoadingChanges(true);
    setChangesError("");
    try {
      setChanges(await api.changes(workspace.id));
    } catch (error) {
      setChangesError((error as Error).message);
    } finally {
      setLoadingChanges(false);
    }
  }

  async function archive() {
    if (await props.controller.archiveWorkspace(workspace)) props.onArchived();
  }

  async function remove() {
    if (await props.controller.deleteWorkspace(workspace)) props.onDeleted();
  }

  async function publish(input: PublishInput) {
    setPublishing(true);
    try {
      const updated = await api.publish(workspace.id, input);
      props.controller.updateWorkspace(updated);
      await loadChanges();
      setPublishOpen(false);
      return true;
    } finally {
      setPublishing(false);
    }
  }

  return (
    <section className="workspace-control-page workspace-overview-page">
      <header className="workspace-page-header">
        <div className="workspace-identity">
          <button className="workspace-back" type="button" onClick={props.onBack}>← <span>Workspaces</span></button>
          <div>
            <h1>{repositoryName(workspace.repository)}</h1>
            <p className="workspace-subtitle">{workspace.repository} <span>·</span> <code>{workspace.branch}</code></p>
          </div>
        </div>
        <div className="workspace-header-state">
          <span className={`status ${workspace.status}`}>{statusLabel(workspace.status)}</span>
          <WorkspaceCapacity workspace={workspace} onManage={props.onManageEnvironments} />
          <button className="primary workspace-conversation-action" type="button" disabled={!conversationReady} onClick={props.onOpenConversation}>
            {props.sessions.length ? "Continue conversation" : "Open conversation"}
          </button>
          <WorkspaceLifecycleMenu
            workspace={workspace}
            deleting={deleting}
            onStop={async () => { await props.controller.workspaceAction(workspace.id, "stop"); }}
            onResume={async () => { await props.controller.workspaceAction(workspace.id, "resume"); }}
            onArchive={archive}
            onDelete={remove}
          />
        </div>
      </header>

      <div className="workspace-overview-scroll">
        <div className="workspace-page-content workspace-overview-content">
          {workspace.status !== "ready" && (
            <WorkspaceReadiness
              workspace={workspace}
              embedded
              onConfigure={(root) => props.controller.configureProjectRoot(workspace.id, root)}
              onRetry={async () => { await props.controller.workspaceAction(workspace.id, "initialize"); }}
              onResume={async () => { await props.controller.workspaceAction(workspace.id, "resume"); }}
              onDelete={remove}
            />
          )}

          {workspace.status === "ready" && (
            <div className="workspace-overview-primary">
              <section className="workspace-overview-section workspace-overview-tasks">
                <div className="workspace-overview-section-heading">
                  <div><p className="eyebrow">Current work</p><h2>Tasks</h2></div>
                  <span>{props.tasks.length}</span>
                </div>
                <WorkspaceTasksPanel embedded tasks={props.tasks} onCreate={props.onCreateTask} onUpdate={props.onUpdateTask} onDelete={props.onDeleteTask} />
              </section>

              <section className="workspace-overview-section changes-summary" aria-label="Workspace changes summary">
                <div className="workspace-overview-section-heading">
                  <div><p className="eyebrow">Working tree</p><h2>Changes</h2></div>
                  <span>{changeCount}</span>
                </div>
                <div className="workspace-overview-changes">
                  {changesError ? (
                    <p role="alert">{changesError}</p>
                  ) : loadingChanges ? (
                    <p>Inspecting the working tree…</p>
                  ) : changeCount ? (
                    <><strong>{changeCount} changed {changeCount === 1 ? "file" : "files"}</strong><p>Review files and publish from this workspace page.</p></>
                  ) : (
                    <><strong>Working tree clean</strong><p>Changes made by the agent will appear here.</p></>
                  )}
                </div>
                <footer>
                  <button className="quiet compact" type="button" disabled={loadingChanges} onClick={() => void loadChanges()}>Refresh</button>
                  <button className="primary compact" type="button" onClick={() => setReviewOpen((current) => !current)}>{reviewOpen ? "Hide review" : "Review changes"}</button>
                </footer>
              </section>
            </div>
          )}

          {workspace.status === "ready" && reviewOpen && (
            <div className="workspace-overview-review">
              <WorkspaceChangesReview
                changes={changes}
                loading={loadingChanges}
                error={changesError}
                onRefresh={() => void loadChanges()}
                onPublish={() => setPublishOpen(true)}
              />
            </div>
          )}

          <section className="workspace-overview-section workspace-facts" aria-label="Workspace details">
            <div className="workspace-overview-section-heading"><div><p className="eyebrow">Workspace</p><h2>Project and environment</h2></div></div>
            <dl>
              <Fact label="Repository" value={workspace.repository} />
              <Fact label="Working branch" value={workspace.branch} code />
              <Fact label="Base branch" value={workspace.base_branch} code />
              <Fact label="Dependencies" value={dependencyStatus(workspace)} />
            </dl>
            <details><summary>Project profile</summary><WorkspaceProfilePanel workspace={workspace} /></details>
            <details><summary>Environment variables</summary><EnvironmentPanel workspace={workspace} sessions={props.sessions} /></details>
          </section>
        </div>
      </div>
      {publishOpen && (
        <div className="workspace-dialog-backdrop" role="presentation" onMouseDown={() => setPublishOpen(false)}>
          <section className="workspace-dialog" role="dialog" aria-modal="true" aria-label="Publish workspace changes" onMouseDown={(event) => event.stopPropagation()}>
            <button className="dialog-close" type="button" aria-label="Close publish dialog" onClick={() => setPublishOpen(false)}>×</button>
            <PublishPanel workspace={workspace} publishing={publishing} onPublish={publish} />
          </section>
        </div>
      )}
    </section>
  );
}

function WorkspaceLifecycleMenu(props: {
  workspace: Workspace;
  deleting: boolean;
  onStop: () => Promise<void>;
  onResume: () => Promise<void>;
  onArchive: () => Promise<void>;
  onDelete: () => Promise<void>;
}) {
  return (
    <details className="workspace-row-menu workspace-lifecycle-menu">
      <summary aria-label="Workspace actions">•••</summary>
      <div>
        {props.workspace.status === "ready" && <button type="button" onClick={() => void props.onStop()}>Stop environment</button>}
        {props.workspace.status === "stopped" && <button type="button" onClick={() => void props.onResume()}>Resume environment</button>}
        <button type="button" disabled={props.deleting} onClick={() => void props.onArchive()}>Archive workspace…</button>
        <button className="danger-text" type="button" disabled={props.deleting} onClick={() => void props.onDelete()}>{props.deleting ? "Deleting…" : "Delete workspace…"}</button>
      </div>
    </details>
  );
}

function Fact(props: { label: string; value: string; code?: boolean }) {
  return <div><dt>{props.label}</dt><dd>{props.code ? <code>{props.value}</code> : props.value}</dd></div>;
}

function dependencyStatus(workspace: Workspace): string {
  const result = workspace.project_profile?.setup_result;
  if (result === "passed") return "Installed";
  if (result === "skipped") return "No setup required";
  if (result === "failed") return "Installation failed";
  if (workspace.preparation_stage === "installing") return "Installing";
  return "Not prepared";
}
