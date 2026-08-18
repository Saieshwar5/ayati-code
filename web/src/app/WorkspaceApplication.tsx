import { useCallback, useEffect, useState } from "react";
import type { User } from "../api/contracts";
import { AgentStudio } from "../agents/AgentStudio";
import { AgentStudioSidebar } from "../agents/AgentStudioSidebar";
import { useAgentController } from "../agents/useAgentController";
import { useSkillController } from "../agents/useSkillController";
import { ChatPane } from "../chat/ChatPane";
import { EnvironmentsPage } from "../environments/EnvironmentsPage";
import { useWorkspaceDetail } from "../hooks/useWorkspaceDetail";
import { useServerEvents } from "../hooks/useServerEvents";
import { Inspector } from "../inspector/Inspector";
import { ArchivedWorkspaces } from "../workspaces/ArchivedWorkspaces";
import { Sidebar } from "../workspaces/Sidebar";
import { WorkspaceHome } from "../workspaces/WorkspaceHome";
import { WorkspaceIndex } from "../workspaces/WorkspaceIndex";
import { WorkspacePage } from "../workspaces/WorkspacePage";
import { isAgentRoute, sessionPath, useAppRoute, workspacePath } from "./useAppRoute";
import { useWorkspaceController } from "./useWorkspaceController";

interface WorkspaceApplicationProps {
  user: User;
}

export function WorkspaceApplication({ user }: WorkspaceApplicationProps) {
  const controller = useWorkspaceController(user);
  const { route, navigate } = useAppRoute();
  const sessionView = route.page === "session";
  const agentStudioView = isAgentRoute(route);
  const agents = useAgentController(agentStudioView || sessionView);
  const skills = useSkillController(agentStudioView);
  const [sidebarCollapsed, setSidebarCollapsedState] = useState(initialSidebarState);
  const [inspectorCollapsed, setInspectorCollapsedState] = useState(initialInspectorState);
  const workspaceID = "workspaceID" in route ? route.workspaceID : "";
  const sessionID = route.page === "session" ? route.sessionID : "";
  const workspace = controller.workspaces.find((item) => item.id === workspaceID);
  const session = (controller.sessions[workspaceID] || []).find((item) => item.id === sessionID);

  useEffect(() => {
    if (workspaceID) void controller.loadSessions(workspaceID);
  }, [controller.loadSessions, workspaceID]);
  const serverEvent = useServerEvents(workspaceID, (changedWorkspaceID) => {
    void controller.loadSessions(changedWorkspaceID);
  });

  const detail = useWorkspaceDetail({
    workspace: route.page === "session" ? workspace : undefined,
    session,
    onSessionUpdate: controller.updateSession,
    onWorkspaceUpdate: controller.updateWorkspace,
    serverEvent,
  });

  const setInspectorCollapsed = useCallback((collapsed: boolean) => {
    setInspectorCollapsedState(collapsed);
    try {
      window.localStorage.setItem("ayati.inspector.collapsed", String(collapsed));
    } catch {
      // The preference is optional when browser storage is unavailable.
    }
  }, []);

  const setSidebarCollapsed = useCallback((collapsed: boolean) => {
    setSidebarCollapsedState(collapsed);
    try {
      window.localStorage.setItem("perpetual.sidebar.collapsed", String(collapsed));
    } catch {
      // The preference is optional when browser storage is unavailable.
    }
  }, []);

  return (
    <main>
      <section className={`app-shell${sidebarCollapsed ? " sidebar-collapsed" : ""}${sessionView ? " session-view" : ""}${agentStudioView ? " agent-studio-view" : ""}${sessionView && inspectorCollapsed ? " inspector-collapsed" : ""}`}>
        <Sidebar controller={controller} route={route} collapsed={sidebarCollapsed} onCollapsedChange={setSidebarCollapsed} onNavigate={navigate} />
        {agentStudioView && <AgentStudioSidebar route={route} agentCount={agents.agents.length} providerCount={agents.providers.length} skillCount={skills.skills.length} onNavigate={navigate} />}
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
          ) : route.page === "workspaces" ? (
            controller.repositoryReconnectRequired && !controller.workspaces.length ? (
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
            ) : (
              <WorkspaceIndex workspaces={controller.workspaces} onCreate={() => navigate("/workspaces/new")} onOpen={(id) => navigate(workspacePath(id))} />
            )
          ) : route.page === "workspace" && workspace ? (
            <WorkspacePage
              workspace={workspace}
              controller={controller}
              onOpenSession={(id) => navigate(sessionPath(workspace.id, id))}
              onManageEnvironments={() => navigate("/environments")}
              onArchived={() => navigate("/workspaces")}
              onDeleted={() => navigate("/workspaces")}
            />
          ) : route.page === "session" && workspace && session ? (
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
            />
          ) : route.page === "archived" ? (
            <ArchivedWorkspaces workspaces={controller.archivedWorkspaces} onRestore={async (id) => { await controller.restoreWorkspace(id); navigate(workspacePath(id)); }} />
          ) : agentStudioView ? (
            <AgentStudio route={route} controller={agents} skills={skills} onNavigate={navigate} />
          ) : route.page === "environments" ? (
            <EnvironmentsPage
              workspaces={controller.workspaces}
              onOpenWorkspace={(id) => navigate(workspacePath(id))}
            />
          ) : (
            <LoadingPage title={workspaceID ? "Workspace not found" : "Session not found"} error />
          )}
        </section>
        {sessionView && (
          <Inspector collapsed={inspectorCollapsed} workspace={workspace} session={session} messages={detail.messages} onCollapsedChange={setInspectorCollapsed} />
        )}
      </section>
    </main>
  );
}

function LoadingPage({ title, error = false }: { title: string; error?: boolean }) {
  return <section className="workspace-home"><div className="workspace-empty" role={error ? "alert" : undefined}><p className="eyebrow">{error ? "Unable to open" : "perpetual"}</p><h1>{title}</h1></div></section>;
}

function initialInspectorState(): boolean {
  let collapsed = typeof window.matchMedia === "function" && window.matchMedia("(max-width: 880px)").matches;
  try {
    const saved = window.localStorage.getItem("ayati.inspector.collapsed");
    if (saved !== null) collapsed = saved === "true";
  } catch {
    // Responsive behavior remains the fallback when storage is unavailable.
  }
  return collapsed;
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
