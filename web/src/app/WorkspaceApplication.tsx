import { useCallback, useEffect, useState } from "react";
import type { User } from "../api/contracts";
import { ChatPane } from "../chat/ChatPane";
import { useWorkspaceDetail } from "../hooks/useWorkspaceDetail";
import { Inspector } from "../inspector/Inspector";
import { ArchivedWorkspaces } from "../workspaces/ArchivedWorkspaces";
import { PlaceholderPage } from "../workspaces/PlaceholderPage";
import { Sidebar } from "../workspaces/Sidebar";
import { WorkspaceHome } from "../workspaces/WorkspaceHome";
import { WorkspaceIndex } from "../workspaces/WorkspaceIndex";
import { WorkspacePage } from "../workspaces/WorkspacePage";
import { sessionPath, useAppRoute, workspacePath } from "./useAppRoute";
import { useWorkspaceController } from "./useWorkspaceController";

interface WorkspaceApplicationProps {
  user: User;
}

export function WorkspaceApplication({ user }: WorkspaceApplicationProps) {
  const controller = useWorkspaceController(user);
  const { route, navigate } = useAppRoute();
  const [inspectorCollapsed, setInspectorCollapsedState] = useState(initialInspectorState);
  const workspaceID = "workspaceID" in route ? route.workspaceID : "";
  const sessionID = route.page === "session" ? route.sessionID : "";
  const workspace = controller.workspaces.find((item) => item.id === workspaceID);
  const session = (controller.sessions[workspaceID] || []).find((item) => item.id === sessionID);

  useEffect(() => {
    if (workspaceID) void controller.loadSessions(workspaceID);
  }, [controller.loadSessions, workspaceID]);

  const detail = useWorkspaceDetail({
    workspace: route.page === "session" ? workspace : undefined,
    session,
    onSessionUpdate: controller.updateSession,
    onWorkspaceUpdate: controller.updateWorkspace,
    onWorkspacesRefresh: controller.refreshWorkspaces,
  });

  const setInspectorCollapsed = useCallback((collapsed: boolean) => {
    setInspectorCollapsedState(collapsed);
    try {
      window.localStorage.setItem("ayati.inspector.collapsed", String(collapsed));
    } catch {
      // The preference is optional when browser storage is unavailable.
    }
  }, []);

  const sessionView = route.page === "session";
  return (
    <main>
      <section className={`app-shell${sessionView ? " session-view" : ""}${sessionView && inspectorCollapsed ? " inspector-collapsed" : ""}`}>
        <Sidebar controller={controller} route={route} onNavigate={navigate} />
        <section className="conversation-pane">
          {controller.loading ? (
            <LoadingPage title="Loading workspaces…" />
          ) : controller.loadError ? (
            <LoadingPage title={controller.loadError} error />
          ) : route.page === "create-workspace" ? (
            <WorkspaceHome
              view="create"
              repositories={controller.repositories}
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
              onSend={detail.sendMessage}
            />
          ) : route.page === "archived" ? (
            <ArchivedWorkspaces workspaces={controller.archivedWorkspaces} onRestore={async (id) => { await controller.restoreWorkspace(id); navigate(workspacePath(id)); }} />
          ) : route.page === "agents" ? (
            <PlaceholderPage eyebrow="Custom behavior" title="Agents" description="Create focused agents with their own instructions and responsibilities. This navigation is ready; agent configuration comes in a separate implementation." />
          ) : route.page === "environments" ? (
            <PlaceholderPage eyebrow="Shared configuration" title="Environments" description="Reusable environment configuration will live here. No environment functionality has been added in this redesign." />
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
  return <section className="workspace-home"><div className="workspace-empty" role={error ? "alert" : undefined}><p className="eyebrow">{error ? "Unable to open" : "Ayati"}</p><h1>{title}</h1></div></section>;
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
