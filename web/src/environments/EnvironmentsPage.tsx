import { useMemo, useState } from "react";
import type { Workspace } from "../api/contracts";
import type { ComputeEnvironment } from "../api/environment-contracts";
import { Icon } from "../ui/Icon";
import { EnvironmentCapacitySummary } from "./EnvironmentCapacitySummary";
import { EnvironmentCreateDrawer } from "./EnvironmentCreateDrawer";
import { EnvironmentDeleteDialog } from "./EnvironmentDeleteDialog";
import { EnvironmentRow } from "./EnvironmentRow";
import { useEnvironmentController } from "./useEnvironmentController";

interface EnvironmentsPageProps {
  workspaces?: Workspace[];
  onOpenWorkspace?: (workspaceID: string) => void;
}

export function EnvironmentsPage({ workspaces = [], onOpenWorkspace }: EnvironmentsPageProps) {
  const controller = useEnvironmentController();
  const [creating, setCreating] = useState(false);
  const [expandedID, setExpandedID] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<ComputeEnvironment | null>(null);
  const environments = useMemo(() => [...controller.environments].sort(compareEnvironments), [controller.environments]);

  return <section className="environment-page-scroll">
    <div className="environment-page-frame">
      <header className="environment-page-heading">
        <div>
          <h1>Environments</h1>
          <p className="muted">Reusable local capacity for active workspaces.</p>
        </div>
        <button className="primary environment-add-button" type="button" aria-expanded={creating} aria-controls="environment-create-drawer" onClick={() => setCreating(true)}>
          <Icon name="plus" />Add environment
        </button>
      </header>

      <EnvironmentCapacitySummary environments={controller.environments} />
      {controller.error && <div className="error" role="alert">{controller.error}</div>}

      {controller.loading ? <p className="muted">Loading environments…</p> : environments.length ? (
        <div className="environment-table">
          <div className="environment-table-header" aria-hidden="true">
            <span>Environment</span><span>Workspace</span><span>Resources</span><span>State</span><span />
          </div>
          {environments.map((value) => <EnvironmentRow
            key={value.id}
            value={value}
            workspace={workspaces.find((workspace) => workspace.id === value.active_lease?.workspace_id)}
            expanded={expandedID === value.id}
            onExpandedChange={(expanded) => setExpandedID(expanded ? value.id : null)}
            onOpenWorkspace={onOpenWorkspace}
            onRepair={async () => {
              try { await controller.repair(value.id); }
              catch (reason) { controller.setError((reason as Error).message); }
            }}
            onRequestDelete={() => setDeleting(value)}
          />)}
        </div>
      ) : <div className="environment-empty-state">
        <Icon name="environments" />
        <h2>No environment capacity</h2>
        <p className="muted">Add a local environment before starting a workspace.</p>
        <button className="primary" type="button" onClick={() => setCreating(true)}>Create environment</button>
      </div>}
    </div>

    {creating && <EnvironmentCreateDrawer
      suggestedName={`Local environment ${controller.environments.length + 1}`}
      onCancel={() => setCreating(false)}
      onCreate={async (input) => {
        await controller.create(input);
        setCreating(false);
      }}
    />}
    {deleting && <EnvironmentDeleteDialog
      environment={deleting}
      onCancel={() => setDeleting(null)}
      onConfirm={async () => {
        await controller.remove(deleting.id);
        setDeleting(null);
      }}
    />}
  </section>;
}

function compareEnvironments(left: ComputeEnvironment, right: ComputeEnvironment): number {
  const rank = (value: ComputeEnvironment) => {
    if (value.quarantined || value.state === "failed") return 0;
    if (value.state === "provisioning" || value.state === "deleting") return 1;
    if (value.state === "occupied" || value.state === "releasing") return 2;
    return 3;
  };
  return rank(left) - rank(right) || left.name.localeCompare(right.name);
}
