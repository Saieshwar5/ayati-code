import { useCallback, useState } from "react";
import type { User } from "../api/contracts";
import { ChatPane } from "../chat/ChatPane";
import { useWorkspaceDetail } from "../hooks/useWorkspaceDetail";
import { Inspector } from "../inspector/Inspector";
import { Sidebar } from "../workspaces/Sidebar";
import { WorkspaceHome } from "../workspaces/WorkspaceHome";
import { WorkspaceReadiness } from "../workspaces/WorkspaceReadiness";
import { useWorkspaceController } from "./useWorkspaceController";

interface WorkspaceApplicationProps {
  user: User;
}

export function WorkspaceApplication({ user }: WorkspaceApplicationProps) {
  const controller = useWorkspaceController(user);
  const [inspectorCollapsed, setInspectorCollapsedState] = useState(initialInspectorState);
  const detail = useWorkspaceDetail({
    workspace: controller.activeWorkspace,
    session: controller.activeSession,
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

  const selectedWorkspace =
    controller.view === "workspace" ? controller.activeWorkspace : undefined;
  const showingWorkspace =
    selectedWorkspace?.status === "ready" && controller.activeSession;

  return (
    <main>
      <section className={`app-shell${inspectorCollapsed ? " inspector-collapsed" : ""}`}>
        <Sidebar controller={controller} />
        <section className="conversation-pane">
          {showingWorkspace ? (
            <ChatPane
              workspace={controller.activeWorkspace!}
              session={controller.activeSession!}
              workspaceSessions={controller.sessions[controller.activeWorkspace!.id] || []}
              messages={detail.messages}
              error={detail.messageError}
              sending={detail.sending}
              onSend={detail.sendMessage}
            />
          ) : selectedWorkspace ? (
            <WorkspaceReadiness
              workspace={selectedWorkspace}
              onConfigure={(root) => controller.configureProjectRoot(selectedWorkspace.id, root)}
              onRetry={() => controller.workspaceAction(selectedWorkspace.id, "initialize")}
              onResume={() => controller.workspaceAction(selectedWorkspace.id, "resume")}
              onDelete={() => controller.deleteWorkspace(selectedWorkspace)}
            />
          ) : controller.loading ? (
            <section className="workspace-home">
              <div className="workspace-empty">
                <p className="eyebrow">Coding workspace</p>
                <h1>Loading workspaces…</h1>
              </div>
            </section>
          ) : controller.loadError ? (
            <section className="workspace-home">
              <div className="workspace-empty" role="alert">
                <p className="eyebrow">Unable to load workspaces</p>
                <h1>{controller.loadError}</h1>
              </div>
            </section>
          ) : (
            <WorkspaceHome
              view={controller.view === "create" ? "create" : "empty"}
              repositories={controller.repositories}
              repositoryError={controller.repositoryError}
              repositoryReconnectRequired={controller.repositoryReconnectRequired}
              onShowCreate={controller.showCreate}
              onCancel={controller.closeCreate}
              onCreate={controller.createWorkspace}
              onCreateProject={controller.createNewProject}
            />
          )}
        </section>
        <Inspector
          collapsed={inspectorCollapsed}
          workspace={selectedWorkspace}
          session={selectedWorkspace ? controller.activeSession : undefined}
          workspaceSessions={
            selectedWorkspace ? controller.sessions[selectedWorkspace.id] || [] : []
          }
          messages={detail.messages}
          changes={detail.changes}
          publishing={detail.publishing}
          onCollapsedChange={setInspectorCollapsed}
          onRefreshChanges={detail.loadChanges}
          onPublish={detail.publish}
          onAuthorityChange={(input) =>
            controller.changeWorkspaceAuthority(selectedWorkspace!.id, input)}
        />
      </section>
    </main>
  );
}

function initialInspectorState(): boolean {
  let collapsed =
    typeof window.matchMedia === "function" && window.matchMedia("(max-width: 880px)").matches;
  try {
    const saved = window.localStorage.getItem("ayati.inspector.collapsed");
    if (saved !== null) collapsed = saved === "true";
  } catch {
    // Responsive behavior remains the fallback when storage is unavailable.
  }
  return collapsed;
}
