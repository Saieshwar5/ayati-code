import { useState } from "react";
import type { ComputeEnvironment, CreateComputeEnvironmentInput } from "../api/environment-contracts";
import { useEnvironmentController } from "./useEnvironmentController";

const initialInput: CreateComputeEnvironmentInput = {
  name: "",
  image_ref: "ayati-sandbox:dev",
  cpu_millis: 2000,
  memory_mb: 4096,
  pid_limit: 256,
  network_policy: "outbound",
};

export function EnvironmentsPage() {
  const controller = useEnvironmentController();
  const [creating, setCreating] = useState(false);
  const available = controller.environments.filter((value) => value.state === "available").length;
  const occupied = controller.environments.filter((value) => value.state === "occupied" || value.state === "releasing").length;
  const attention = controller.environments.filter((value) => value.state === "failed").length;

  return <section className="environment-page-scroll">
    <div className="environment-page-frame">
      <header className="environment-page-heading">
        <div>
          <p className="eyebrow">Reusable local compute</p>
          <h1>Environments</h1>
          <p className="muted">Each active workspace exclusively leases one prepared environment. Stopping it returns that capacity to this pool.</p>
        </div>
        <button className="primary" type="button" onClick={() => setCreating(true)}>＋ New environment</button>
      </header>

      <div className="environment-summary" aria-label="Environment capacity summary">
        <Summary label="Available" value={available} tone="available" />
        <Summary label="In use" value={occupied} tone="occupied" />
        <Summary label="Needs attention" value={attention} tone="failed" />
        <Summary label="Total capacity" value={controller.environments.length} tone="total" />
      </div>

      {controller.error && <div className="error" role="alert">{controller.error}</div>}
      {creating && <EnvironmentForm
        onCancel={() => setCreating(false)}
        onCreate={async (input) => {
          await controller.create(input);
          setCreating(false);
        }}
      />}

      {controller.loading ? <p className="muted">Loading environments…</p> : controller.environments.length ? (
        <div className="compute-card-grid">
          {controller.environments.map((value) => <EnvironmentCard
            key={value.id}
            value={value}
            onRepair={async () => {
              try { await controller.repair(value.id); }
              catch (reason) { controller.setError((reason as Error).message); }
            }}
            onDelete={async () => {
              if (!window.confirm(`Delete “${value.name}”?\n\nOnly its reusable compute configuration will be removed.`)) return;
              try { await controller.remove(value.id); }
              catch (reason) { controller.setError((reason as Error).message); }
            }}
          />)}
        </div>
      ) : <div className="environment-empty-state">
        <span aria-hidden="true">⌁</span>
        <h2>No environment capacity</h2>
        <p className="muted">Create a local Docker environment before starting a workspace.</p>
        <button className="primary" type="button" onClick={() => setCreating(true)}>Add environment</button>
      </div>}

      <aside className="environment-boundary-note">
        <strong>Controller-owned boundary</strong>
        <p>Ayati resolves the image to an immutable identity and controls leases and containers. Agents receive only the bounded shell inside the leased runtime.</p>
      </aside>
    </div>
  </section>;
}

function Summary(props: { label: string; value: number; tone: string }) {
  return <div className={`environment-summary-item ${props.tone}`}>
    <span>{props.label}</span><strong>{props.value}</strong>
  </div>;
}

function EnvironmentCard(props: {
  value: ComputeEnvironment;
  onRepair: () => Promise<void>;
  onDelete: () => Promise<void>;
}) {
  const [busy, setBusy] = useState<"repair" | "delete" | "">("");
  const value = props.value;
  const occupied = value.state === "occupied" || value.state === "releasing";
  async function run(action: "repair" | "delete", callback: () => Promise<void>) {
    setBusy(action);
    try { await callback(); } finally { setBusy(""); }
  }
  return <article className={`compute-card ${value.state}`}>
    <header>
      <span className="compute-mark" aria-hidden="true">⌁</span>
      <span className={`compute-state ${value.quarantined ? "failed" : value.state}`}>{stateLabel(value)}</span>
    </header>
    <div className="compute-card-copy">
      <h2>{value.name}</h2>
      <code>{value.image_ref}</code>
      <p>{formatCPU(value.cpu_millis)} · {formatMemory(value.memory_mb)} · {value.pid_limit} processes</p>
    </div>
    <dl className="compute-details">
      <div><dt>Network</dt><dd>{value.network_policy === "outbound" ? "Outbound" : "Disabled"}</dd></div>
      <div><dt>Generation</dt><dd>{value.generation}</dd></div>
      <div><dt>Driver</dt><dd>Docker</dd></div>
    </dl>
    {value.active_lease && <div className="compute-lease">
      <span>Leased workspace</span>
      <strong title={value.active_lease.workspace_id}>{shortID(value.active_lease.workspace_id)}</strong>
    </div>}
    {value.error && <p className="compute-error" role="alert">{value.error}</p>}
    {value.quarantined && <p className="compute-quarantine">Delete the failed workspace before repairing or removing this capacity.</p>}
    <footer>
      {!value.quarantined && (value.state === "failed" || value.state === "provisioning") && <button className="quiet compact" type="button" disabled={busy !== ""} onClick={() => void run("repair", props.onRepair)}>{busy === "repair" ? "Repairing…" : "Repair"}</button>}
      <button className="quiet compact danger-text" type="button" disabled={occupied || value.quarantined || busy !== ""} title={environmentDeleteTitle(value)} onClick={() => void run("delete", props.onDelete)}>{busy === "delete" ? "Deleting…" : "Delete"}</button>
    </footer>
  </article>;
}

function EnvironmentForm(props: {
  onCancel: () => void;
  onCreate: (input: CreateComputeEnvironmentInput) => Promise<void>;
}) {
  const [input, setInput] = useState(initialInput);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function submit(event: React.FormEvent) {
    event.preventDefault(); setBusy(true); setError("");
    try { await props.onCreate(input); }
    catch (reason) { setError((reason as Error).message); }
    finally { setBusy(false); }
  }
  return <form className="compute-form" onSubmit={(event) => void submit(event)}>
    <header><div><p className="eyebrow">Add capacity</p><h2>New local environment</h2></div><button className="quiet compact" type="button" onClick={props.onCancel}>Close</button></header>
    <div className="compute-form-grid">
      <label>Name<input required maxLength={80} value={input.name} placeholder="General coding" onChange={(event) => setInput({ ...input, name: event.target.value })} /></label>
      <label className="wide">Docker image<input required maxLength={512} value={input.image_ref} onChange={(event) => setInput({ ...input, image_ref: event.target.value })} /></label>
      <label>CPU cores<input required type="number" min="0.1" max="64" step="0.1" value={input.cpu_millis / 1000} onChange={(event) => setInput({ ...input, cpu_millis: Math.round(Number(event.target.value) * 1000) })} /></label>
      <label>Memory (MB)<input required type="number" min="128" max="262144" value={input.memory_mb} onChange={(event) => setInput({ ...input, memory_mb: Number(event.target.value) })} /></label>
      <label>Process limit<input required type="number" min="16" max="65535" value={input.pid_limit} onChange={(event) => setInput({ ...input, pid_limit: Number(event.target.value) })} /></label>
      <label>Network<select value={input.network_policy} onChange={(event) => setInput({ ...input, network_policy: event.target.value as "disabled" | "outbound" })}><option value="outbound">Outbound access</option><option value="disabled">Disabled</option></select></label>
    </div>
    <p className="compute-form-note">The image must already exist locally. Ayati records its resolved immutable image identity.</p>
    {error && <div className="error" role="alert">{error}</div>}
    <div className="compute-form-actions"><button className="quiet" type="button" onClick={props.onCancel}>Cancel</button><button className="primary" type="submit" disabled={busy}>{busy ? "Preparing…" : "Create environment"}</button></div>
  </form>;
}

function stateLabel(value: ComputeEnvironment): string {
  if (value.quarantined) return "Quarantined";
  return ({ available: "Available", occupied: "In use", releasing: "Releasing", failed: "Needs attention", provisioning: "Preparing", deleting: "Deleting" })[value.state];
}

function environmentDeleteTitle(value: ComputeEnvironment): string | undefined {
  if (value.quarantined) return "Delete the failed workspace before removing this environment";
  if (value.state === "occupied" || value.state === "releasing") return "Stop the workspace before deleting this environment";
  return undefined;
}

function shortID(value: string): string { return value.length > 12 ? value.slice(0, 12) : value; }
function formatCPU(millis: number): string { return `${Number((millis / 1000).toFixed(1))} CPU`; }
function formatMemory(megabytes: number): string { return megabytes >= 1024 ? `${Number((megabytes / 1024).toFixed(1))} GB` : `${megabytes} MB`; }
