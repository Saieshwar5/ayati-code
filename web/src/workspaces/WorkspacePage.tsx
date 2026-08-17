import { useEffect, useState } from "react";
import type { PublishInput, Workspace, WorkspaceSession } from "../api/contracts";
import { api } from "../api/client";
import { repositoryName, sessionMeta, statusLabel } from "../app/format";
import { EnvironmentPanel } from "../inspector/EnvironmentPanel";
import { PublishPanel } from "../inspector/PublishPanel";
import { WorkspaceProfilePanel } from "../inspector/WorkspaceProfilePanel";
import type { WorkspaceController } from "../app/useWorkspaceController";
import { WorkspaceCapacity } from "./WorkspaceCapacity";
import { WorkspaceReadiness } from "./WorkspaceReadiness";

type WorkspaceTab = "sessions" | "overview" | "changes" | "environment" | "settings";

interface WorkspacePageProps {
  workspace: Workspace;
  controller: WorkspaceController;
  onOpenSession: (sessionID: string) => void;
  onManageEnvironments: () => void;
  onArchived: () => void;
  onDeleted: () => void;
}

export function WorkspacePage(props: WorkspacePageProps) {
  const { workspace, controller } = props;
  const [tab, setTab] = useState<WorkspaceTab>("sessions");
  const [changes, setChanges] = useState("Select refresh to inspect the current working tree.");
  const [publishing, setPublishing] = useState(false);
  const sessions = controller.sessions[workspace.id] || [];

  useEffect(() => {
    void controller.loadSessions(workspace.id);
  }, [controller.loadSessions, workspace.id]);

  async function createSession() {
    const created = await controller.createSession(workspace.id);
    props.onOpenSession(created.id);
  }

  async function loadChanges() {
    setChanges("Loading changes…");
    try {
      const value = await api.changes(workspace.id);
      setChanges([value.status, value.diff].filter(Boolean).join("\n") || "Working tree is clean.");
    } catch (error) {
      setChanges((error as Error).message);
    }
  }

  async function publish(input: PublishInput) {
    setPublishing(true);
    try {
      const updated = await api.publish(workspace.id, input);
      controller.updateWorkspace(updated);
      await loadChanges();
      return true;
    } finally {
      setPublishing(false);
    }
  }

  async function archive() {
    if (await controller.archiveWorkspace(workspace)) props.onArchived();
  }

  async function remove() {
    if (await controller.deleteWorkspace(workspace)) props.onDeleted();
  }

  return (
    <section className="workspace-control-page">
      <header className="workspace-page-header">
        <div className="workspace-identity">
          <button className="workspace-avatar" type="button" aria-label="Workspace overview" onClick={() => setTab("overview")}>{repositoryName(workspace.repository).slice(0, 1).toUpperCase()}</button>
          <div>
            <p className="eyebrow">{workspace.repository}</p>
            <h1>{repositoryName(workspace.repository)}</h1>
            <p className="workspace-subtitle">{workspace.branch} · {workspace.authority === "explore" ? "Protected Explore" : "Develop"}</p>
          </div>
        </div>
        <div className="workspace-header-state">
          <WorkspaceCapacity workspace={workspace} onManage={props.onManageEnvironments} />
          <span className={`status ${workspace.status}`}>{statusLabel(workspace.status)}</span>
          {workspace.status === "ready" && <button className="quiet compact" type="button" onClick={() => void controller.workspaceAction(workspace.id, "stop")}>Stop</button>}
          {workspace.status === "stopped" && <button className="quiet compact" type="button" onClick={() => void controller.workspaceAction(workspace.id, "resume")}>Resume</button>}
        </div>
      </header>

      <nav className="workspace-tabs" aria-label="Workspace sections">
        <Tab name="sessions" selected={tab} onSelect={setTab} />
        <Tab name="overview" selected={tab} onSelect={setTab} />
        <Tab name="changes" selected={tab} onSelect={(next) => { setTab(next); void loadChanges(); }} />
        <Tab name="environment" selected={tab} onSelect={setTab} />
        <Tab name="settings" selected={tab} onSelect={setTab} />
      </nav>

      <div className="workspace-page-content">
        {workspace.status !== "ready" && tab === "sessions" ? (
          <WorkspaceReadiness
            workspace={workspace}
            embedded
            onConfigure={(root) => controller.configureProjectRoot(workspace.id, root)}
            onRetry={() => controller.workspaceAction(workspace.id, "initialize")}
            onResume={() => controller.workspaceAction(workspace.id, "resume")}
            onDelete={remove}
          />
        ) : tab === "sessions" ? (
          <SessionsPanel
            sessions={sessions}
            onCreate={createSession}
            onOpen={props.onOpenSession}
            onRename={(session) => controller.renameSession(workspace.id, session)}
            onDelete={(session) => controller.deleteSession(workspace.id, session)}
          />
        ) : tab === "overview" ? (
          <div className="workspace-tool-panel"><WorkspaceProfilePanel workspace={workspace} agentWorking={sessions.some((session) => session.status === "working")} onAuthorityChange={(input) => controller.changeWorkspaceAuthority(workspace.id, input)} /></div>
        ) : tab === "changes" ? (
          <div className="workspace-two-column">
            <section className="workspace-tool-card">
              <div className="section-heading"><div><p className="eyebrow">Review</p><h2>Workspace changes</h2></div><button className="quiet compact" type="button" onClick={() => void loadChanges()}>Refresh</button></div>
              <p className="scope-note">Changes are shared by every session in this workspace.</p>
              <pre className="changes-output">{changes}</pre>
            </section>
            <div className="workspace-tool-card"><PublishPanel workspace={workspace} publishing={publishing} onPublish={publish} /></div>
          </div>
        ) : tab === "environment" ? (
          <div className="workspace-tool-panel"><EnvironmentPanel workspace={workspace} sessions={sessions} /></div>
        ) : (
          <SettingsPanel workspace={workspace} onArchive={archive} onDelete={remove} />
        )}
      </div>
    </section>
  );
}

function Tab(props: { name: WorkspaceTab; selected: WorkspaceTab; onSelect: (tab: WorkspaceTab) => void }) {
  return <button className={props.selected === props.name ? "active" : ""} type="button" aria-current={props.selected === props.name ? "page" : undefined} onClick={() => props.onSelect(props.name)}>{props.name[0].toUpperCase() + props.name.slice(1)}</button>;
}

function SessionsPanel(props: {
  sessions: WorkspaceSession[];
  onCreate: () => Promise<void>;
  onOpen: (id: string) => void;
  onRename: (session: WorkspaceSession) => Promise<void>;
  onDelete: (session: WorkspaceSession) => Promise<boolean>;
}) {
  return (
    <section className="sessions-panel">
      <div className="section-heading"><div><p className="eyebrow">Conversations</p><h2>Sessions</h2><p className="muted">Each session has its own conversation. All sessions share this workspace’s files.</p></div><button className="primary" type="button" onClick={() => void props.onCreate()}>＋ New session</button></div>
      <div className="workspace-session-list">
        {props.sessions.map((session) => (
          <article className={`workspace-session-card ${session.status}`} key={session.id}>
            <button className="session-card-open" type="button" onClick={() => props.onOpen(session.id)}>
              <span className="session-status-dot" aria-hidden="true" />
              <span><strong>{session.title}</strong><small>{sessionMeta(session)}</small></span>
            </button>
            <div className="session-card-actions">
              <button className="quiet compact" type="button" onClick={() => void props.onRename(session)}>Rename</button>
              <button className="quiet compact danger-text" type="button" onClick={() => void props.onDelete(session)}>Delete</button>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}

function SettingsPanel(props: { workspace: Workspace; onArchive: () => Promise<void>; onDelete: () => Promise<void> }) {
  return (
    <section className="settings-panel">
      <div><p className="eyebrow">Lifecycle</p><h2>Workspace settings</h2><p className="muted">Archive preserves the repository and sessions. Delete permanently removes the local workspace.</p></div>
      <article className="setting-row"><div><strong>Archive workspace</strong><p>Stop the sandbox and move this workspace out of active navigation.</p></div><button className="quiet" type="button" onClick={() => void props.onArchive()}>Archive…</button></article>
      <article className="setting-row danger-zone"><div><strong>Delete workspace</strong><p>This permanently removes local files, sessions, and conversation history.</p></div><button className="quiet danger-text" type="button" onClick={() => void props.onDelete()}>Delete…</button></article>
    </section>
  );
}
