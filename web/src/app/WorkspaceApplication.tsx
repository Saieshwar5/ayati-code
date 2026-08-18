import { useCallback, useEffect, useState } from "react";
import type { User } from "../api/contracts";
import { AgentStudio } from "../agents/AgentStudio";
import { useAgentController } from "../agents/useAgentController";
import { useSkillController } from "../agents/useSkillController";
import { ChatPane } from "../chat/ChatPane";
import { EnvironmentsPage } from "../environments/EnvironmentsPage";
import { useWorkspaceDetail } from "../hooks/useWorkspaceDetail";
import { useServerEvents } from "../hooks/useServerEvents";
import { Sidebar } from "../workspaces/Sidebar";
import { WorkspaceHome } from "../workspaces/WorkspaceHome";
import { WorkspaceIndex } from "../workspaces/WorkspaceIndex";
import { WorkspacePanel, type WorkspacePanelSection } from "../workspaces/WorkspacePanel";
import { taskMarkdownFromRequest, type WorkspaceTask } from "../workspaces/WorkspaceTasksPanel";
import { isAgentRoute, useAppRoute, workspacePath } from "./useAppRoute";
import { useWorkspaceController } from "./useWorkspaceController";

interface WorkspaceApplicationProps {
  user: User;
}

export function WorkspaceApplication({ user }: WorkspaceApplicationProps) {
  const controller = useWorkspaceController(user);
  const { route, navigate } = useAppRoute();
  const workspaceChatView = route.page === "workspace";
  const agentStudioView = isAgentRoute(route);
  const agents = useAgentController(agentStudioView || workspaceChatView);
  const skills = useSkillController(agentStudioView);
  const [sidebarCollapsed, setSidebarCollapsedState] = useState(initialSidebarState);
  const [tasksByWorkspace, setTasksByWorkspace] = useState<Record<string, WorkspaceTask[]>>({});
  const [workspacePanelOpen, setWorkspacePanelOpen] = useState(false);
  const [workspacePanelExpanded, setWorkspacePanelExpanded] = useState(false);
  const [workspacePanelSection, setWorkspacePanelSection] = useState<WorkspacePanelSection>("tasks");
  const workspaceID = "workspaceID" in route ? route.workspaceID : "";
  const workspace = controller.workspaces.find((item) => item.id === workspaceID);
  const workspaceSessions = controller.sessions[workspaceID] || [];
  const session = workspaceChatView
    ? workspaceSessions.find((item) => item.status === "working") || [...workspaceSessions].sort((left, right) => Date.parse(right.updated_at) - Date.parse(left.updated_at))[0]
    : undefined;

  useEffect(() => {
    if (!workspaceID) return;
    void controller.loadSessions(workspaceID).then(async (values) => {
      if (route.page === "workspace" && !values.length) await controller.createSession(workspaceID);
    });
  }, [controller.createSession, controller.loadSessions, route.page, workspaceID]);

  useEffect(() => {
    setWorkspacePanelOpen(false);
    setWorkspacePanelExpanded(false);
    setWorkspacePanelSection("tasks");
  }, [workspaceID]);
  const serverEvent = useServerEvents(workspaceID, (changedWorkspaceID) => {
    void controller.loadSessions(changedWorkspaceID);
  });

  const detail = useWorkspaceDetail({
    workspace: workspaceChatView ? workspace : undefined,
    session,
    onSessionUpdate: controller.updateSession,
    onWorkspaceUpdate: controller.updateWorkspace,
    serverEvent,
  });

  const setSidebarCollapsed = useCallback((collapsed: boolean) => {
    setSidebarCollapsedState(collapsed);
    try {
      window.localStorage.setItem("perpetual.sidebar.collapsed", String(collapsed));
    } catch {
      // The preference is optional when browser storage is unavailable.
    }
  }, []);

  const createTask = useCallback((workspaceID: string, markdown: string) => {
    setTasksByWorkspace((current) => ({
      ...current,
      [workspaceID]: [
        ...(current[workspaceID] || []),
        { id: `task-${Date.now()}-${current[workspaceID]?.length || 0}`, markdown, status: "ready" },
      ],
    }));
  }, []);

  const updateTask = useCallback((workspaceID: string, task: WorkspaceTask) => {
    setTasksByWorkspace((current) => ({
      ...current,
      [workspaceID]: (current[workspaceID] || []).map((item) => item.id === task.id ? task : item),
    }));
  }, []);

  const deleteTask = useCallback((workspaceID: string, taskID: string) => {
    setTasksByWorkspace((current) => ({
      ...current,
      [workspaceID]: (current[workspaceID] || []).filter((item) => item.id !== taskID),
    }));
  }, []);

  return (
    <main>
      <section className={`app-shell${sidebarCollapsed ? " sidebar-collapsed" : ""}${workspaceChatView ? " workspace-chat-view" : ""}${workspacePanelOpen ? " workspace-panel-open" : ""}${workspacePanelExpanded ? " workspace-panel-expanded" : ""}${agentStudioView ? " agent-studio-view" : ""}`}>
        <Sidebar controller={controller} route={route} collapsed={sidebarCollapsed} onCollapsedChange={setSidebarCollapsed} onNavigate={navigate} />
        <section className="conversation-pane">
          {controller.loading ? (
            <LoadingPage title="Loading workspaces…" />
          ) : controller.loadError ? (
            <LoadingPage title={controller.loadError} error />
          ) : route.page === "create-workspace" ? (
            <WorkspaceHome
              view="create"
              repositories={controller.repositories}
              recentRepositories={[...controller.workspaces]
                .sort((left, right) => Date.parse(right.updated_at) - Date.parse(left.updated_at))
                .map((workspace) => workspace.repository)}
              repositoryError={controller.repositoryError}
              repositoryReconnectRequired={controller.repositoryReconnectRequired}
              onShowCreate={() => navigate("/workspaces/new")}
              onCancel={() => navigate("/workspaces")}
              onCreate={async (input) => {
                const created = await controller.createWorkspace(input);
                navigate(workspacePath(created.id));
              }}
              onCreateProject={async (input) => {
                const created = await controller.createNewProject(input);
                navigate(workspacePath(created.id));
              }}
            />
          ) : route.page === "workspaces" && controller.repositoryReconnectRequired && !controller.workspaces.length ? (
            <WorkspaceHome
              view="empty"
              repositories={controller.repositories}
              repositoryError={controller.repositoryError}
              repositoryReconnectRequired
              onShowCreate={() => navigate("/workspaces/new")}
              onCancel={() => navigate("/workspaces")}
              onCreate={async () => {}}
              onCreateProject={async () => {}}
            />
          ) : route.page === "workspaces" || route.page === "archived" ? (
            <WorkspaceIndex
              workspaces={controller.workspaces}
              archivedWorkspaces={controller.archivedWorkspaces}
              view={route.page === "archived" ? "archived" : "active"}
              onViewChange={(view) => navigate(view === "archived" ? "/workspaces/archived" : "/workspaces")}
              onCreate={() => navigate("/workspaces/new")}
              onOpen={(id) => navigate(workspacePath(id))}
              onStop={(workspace) => controller.workspaceAction(workspace.id, "stop")}
              onResume={(workspace) => controller.workspaceAction(workspace.id, "resume")}
              onArchive={controller.archiveWorkspace}
              onRestore={async (workspace) => { await controller.restoreWorkspace(workspace.id); }}
              onDelete={controller.deleteWorkspace}
              deletingWorkspaceIDs={controller.deletingWorkspaceIDs}
            />
          ) : route.page === "workspace" && workspace && session ? (
            <ChatPane
              workspace={workspace}
              session={session}
              workspaceSessions={controller.sessions[workspace.id] || []}
              messages={detail.messages}
              error={detail.messageError}
              sending={detail.sending}
              stopping={detail.stopping}
              agents={agents.agents}
              onSend={detail.sendMessage}
              onStop={detail.stopRun}
              onSelectAgent={async (agentID) => {
                await controller.selectSessionAgent(workspace.id, session.id, agentID);
              }}
              onCreateTask={(request) => createTask(workspace.id, taskMarkdownFromRequest(request))}
              onResumeWorkspace={() => void controller.workspaceAction(workspace.id, "resume")}
            />
          ) : route.page === "workspace" && workspace ? (
            <LoadingPage title="Opening conversation…" />
          ) : agentStudioView ? (
            <AgentStudio route={route} controller={agents} skills={skills} onNavigate={navigate} />
          ) : route.page === "environments" ? (
            <EnvironmentsPage
              workspaces={controller.workspaces}
              onOpenWorkspace={(id) => navigate(workspacePath(id))}
            />
          ) : (
            <LoadingPage title={workspaceID ? "Workspace not found" : "Conversation not found"} error />
          )}
        </section>
        {workspaceChatView && workspace && (
          <WorkspacePanel
            workspace={workspace}
            controller={controller}
            open={workspacePanelOpen}
            section={workspacePanelSection}
            expanded={workspacePanelExpanded}
            tasks={tasksByWorkspace[workspace.id] || []}
            onOpenChange={setWorkspacePanelOpen}
            onSectionChange={setWorkspacePanelSection}
            onExpandedChange={setWorkspacePanelExpanded}
            onCreateTask={(markdown) => createTask(workspace.id, markdown)}
            onUpdateTask={(task) => updateTask(workspace.id, task)}
            onDeleteTask={(taskID) => deleteTask(workspace.id, taskID)}
            onManageEnvironments={() => navigate("/environments")}
            onArchived={() => navigate("/workspaces")}
            onDeleted={() => navigate("/workspaces")}
          />
        )}
      </section>
    </main>
  );
}

function LoadingPage({ title, error = false }: { title: string; error?: boolean }) {
  return <section className="workspace-home"><div className="workspace-empty" role={error ? "alert" : undefined}><p className="eyebrow">{error ? "Unable to open" : "perpetual"}</p><h1>{title}</h1></div></section>;
}

function initialSidebarState(): boolean {
  let collapsed = typeof window.matchMedia === "function" && window.matchMedia("(max-width: 620px)").matches;
  try {
    const saved = window.localStorage.getItem("perpetual.sidebar.collapsed");
    if (saved !== null) collapsed = saved === "true";
  } catch {
    // Responsive behavior remains the fallback when storage is unavailable.
  }
  return collapsed;
}
