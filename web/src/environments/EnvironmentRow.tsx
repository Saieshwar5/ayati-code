import { useState } from "react";
import type { Workspace } from "../api/contracts";
import type { ComputeEnvironment } from "../api/environment-contracts";
import { repositoryName } from "../app/format";
import { Icon } from "../ui/Icon";

interface EnvironmentRowProps {
  value: ComputeEnvironment;
  workspace?: Workspace;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
  onOpenWorkspace?: (workspaceID: string) => void;
  onRepair: () => Promise<void>;
  onRequestDelete: () => void;
}

export function EnvironmentRow(props: EnvironmentRowProps) {
  const [repairing, setRepairing] = useState(false);
  const value = props.value;
  const occupied = value.state === "occupied" || value.state === "releasing";
  const canRepair = !value.quarantined && (value.state === "failed" || value.state === "provisioning");
  const canDelete = !occupied && !value.quarantined && value.state !== "deleting";
  const detailsID = `environment-details-${value.id}`;

  async function repair() {
    setRepairing(true);
    try { await props.onRepair(); }
    finally { setRepairing(false); }
  }

  return <article className={`environment-row ${stateTone(value)}${props.expanded ? " expanded" : ""}`}>
    <div className="environment-row-main">
      <button className="environment-row-identity" type="button" aria-expanded={props.expanded} aria-controls={detailsID} onClick={() => props.onExpandedChange(!props.expanded)}>
        <span className="environment-state-dot" aria-hidden="true" />
        <span className="environment-row-copy">
          <h2>{value.name}</h2>
          <code>{value.image_ref}</code>
          {value.error && <small className="environment-row-error">{value.error}</small>}
        </span>
        <span className="environment-row-chevron" aria-hidden="true" />
      </button>
      <LeaseOwner value={value} workspace={props.workspace} onOpenWorkspace={props.onOpenWorkspace} />
      <span className="environment-row-resources"><strong>{formatCPU(value.cpu_millis)} · {formatMemory(value.memory_mb)}</strong><small>{value.pid_limit} processes</small></span>
      <span className="environment-row-state">{stateLabel(value)}</span>
      <div className="environment-row-actions">
        {canRepair && <button className="quiet compact" type="button" disabled={repairing} onClick={() => void repair()}>{repairing ? "Repairing…" : "Repair"}</button>}
        {value.state !== "deleting" && <details className="environment-row-menu">
          <summary aria-label={`Actions for ${value.name}`}><Icon name="more" /></summary>
          <div className="context-menu">
            <button type="button" onClick={() => props.onExpandedChange(!props.expanded)}>{props.expanded ? "Hide details" : "View details"}</button>
            {!value.quarantined && <button className="danger-text" type="button" disabled={!canDelete} title={environmentDeleteTitle(value)} onClick={props.onRequestDelete}>Delete</button>}
          </div>
        </details>}
      </div>
    </div>

    {props.expanded && <div className="environment-row-details" id={detailsID}>
      <dl>
        <Detail label="Driver" value="Local" />
        <Detail label="Network" value={value.network_policy === "outbound" ? "Outbound" : "Disabled"} />
        <Detail label="Process limit" value={String(value.pid_limit)} />
        <Detail label="Generation" value={String(value.generation)} />
        {value.active_lease && <Detail label="Lease" value={`${value.active_lease.state} · generation ${value.active_lease.generation}`} />}
        {value.image_digest && <Detail label="Image identity" value={value.image_digest} code />}
      </dl>
      {value.quarantined && <div className="environment-row-callout"><strong>Blocked by a failed workspace</strong><p>Delete the failed workspace before repairing or removing this environment.</p></div>}
      {value.error && <div className="environment-row-callout" role="alert"><strong>Environment error</strong><p>{value.error}</p></div>}
    </div>}
  </article>;
}

function LeaseOwner(props: Pick<EnvironmentRowProps, "workspace" | "onOpenWorkspace"> & { value: ComputeEnvironment }) {
  const lease = props.value.active_lease;
  if (!lease) return <span className="environment-row-workspace empty">Not assigned</span>;
  const name = props.workspace ? repositoryName(props.workspace.repository) : shortID(lease.workspace_id);
  if (props.workspace && props.onOpenWorkspace) {
    return <button className="environment-row-workspace linked" type="button" aria-label={`Open workspace ${props.workspace.repository}`} onClick={() => props.onOpenWorkspace?.(props.workspace!.id)}><strong>{name}</strong><small>{props.workspace.branch}</small></button>;
  }
  return <span className="environment-row-workspace"><strong title={props.workspace?.repository || lease.workspace_id}>{name}</strong>{props.workspace && <small>{props.workspace.branch}</small>}</span>;
}

function Detail(props: { label: string; value: string; code?: boolean }) {
  return <div><dt>{props.label}</dt><dd className={props.code ? "code" : undefined} title={props.value}>{props.value}</dd></div>;
}

function stateLabel(value: ComputeEnvironment): string {
  if (value.quarantined) return "Blocked";
  return ({ available: "Available", occupied: "In use", releasing: "Releasing", failed: "Needs repair", provisioning: "Preparing", deleting: "Deleting" })[value.state];
}

function stateTone(value: ComputeEnvironment): string {
  if (value.quarantined || value.state === "failed") return "attention";
  if (value.state === "occupied" || value.state === "releasing") return "occupied";
  if (value.state === "available") return "available";
  return "updating";
}

function environmentDeleteTitle(value: ComputeEnvironment): string | undefined {
  if (value.state === "occupied" || value.state === "releasing") return "Stop the workspace before deleting this environment";
  return undefined;
}

function shortID(value: string): string { return value.length > 12 ? value.slice(0, 12) : value; }
function formatCPU(millis: number): string { return `${Number((millis / 1000).toFixed(1))} CPU`; }
function formatMemory(megabytes: number): string { return megabytes >= 1024 ? `${Number((megabytes / 1024).toFixed(1))} GB` : `${megabytes} MB`; }
