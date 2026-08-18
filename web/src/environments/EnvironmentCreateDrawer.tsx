import { useState } from "react";
import type { CreateComputeEnvironmentInput } from "../api/environment-contracts";

const defaultInput: CreateComputeEnvironmentInput = {
  name: "",
  image_ref: "perpetual-sandbox:dev",
  cpu_millis: 2000,
  memory_mb: 4096,
  pid_limit: 256,
  network_policy: "outbound",
};

interface EnvironmentCreateDrawerProps {
  suggestedName: string;
  onCancel: () => void;
  onCreate: (input: CreateComputeEnvironmentInput) => Promise<void>;
}

export function EnvironmentCreateDrawer(props: EnvironmentCreateDrawerProps) {
  const [input, setInput] = useState(defaultInput);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try { await props.onCreate(input); }
    catch (reason) { setError((reason as Error).message); }
    finally { setBusy(false); }
  }

  return <aside className="environment-create-drawer" id="environment-create-drawer" aria-label="Add environment">
    <form onSubmit={(event) => void submit(event)}>
      <header>
        <div><p className="eyebrow">Local capacity</p><h2>Add environment</h2></div>
        <button className="quiet compact" type="button" disabled={busy} onClick={props.onCancel}>Close</button>
      </header>
      <div className="environment-create-body">
        <label>Name<input required autoFocus maxLength={80} value={input.name} placeholder={props.suggestedName} onChange={(event) => setInput({ ...input, name: event.target.value })} /></label>
        <label>Docker image<input required maxLength={512} value={input.image_ref} onChange={(event) => setInput({ ...input, image_ref: event.target.value })} /></label>

        <details className="environment-resource-disclosure">
          <summary>
            <span><strong>Resources</strong><small>{formatCPU(input.cpu_millis)} · {formatMemory(input.memory_mb)} · {input.pid_limit} processes · {input.network_policy === "outbound" ? "Outbound" : "No network"}</small></span>
            <em>Customize</em>
          </summary>
          <div className="environment-resource-fields">
            <label>CPU cores<input required type="number" min="0.1" max="64" step="0.1" value={input.cpu_millis / 1000} onChange={(event) => setInput({ ...input, cpu_millis: Math.round(Number(event.target.value) * 1000) })} /></label>
            <label>Memory (MB)<input required type="number" min="128" max="262144" value={input.memory_mb} onChange={(event) => setInput({ ...input, memory_mb: Number(event.target.value) })} /></label>
            <label>Process limit<input required type="number" min="16" max="65535" value={input.pid_limit} onChange={(event) => setInput({ ...input, pid_limit: Number(event.target.value) })} /></label>
            <label className="environment-network-switch">
              <input type="checkbox" role="switch" checked={input.network_policy === "outbound"} onChange={(event) => setInput({ ...input, network_policy: event.target.checked ? "outbound" : "disabled" })} />
              <span aria-hidden="true"><i /></span>
              <span><strong>Outbound network</strong><small>Allow dependency downloads.</small></span>
            </label>
          </div>
        </details>

        <div className="environment-controller-note">
          <strong>Controller-owned</strong>
          <p>Perpetual resolves the local image and controls leases and containers. Agents receive only the bounded workspace shell.</p>
        </div>
        <p className="environment-image-note">The Docker image must already exist on this machine.</p>
        {busy && <p className="environment-create-progress" role="status">Resolving Docker image…</p>}
        {error && <div className="error" role="alert">{error}</div>}
      </div>
      <footer><button className="quiet" type="button" disabled={busy} onClick={props.onCancel}>Cancel</button><button className="primary" type="submit" disabled={busy || !input.name.trim() || !input.image_ref.trim()}>{busy ? "Preparing…" : "Create environment"}</button></footer>
    </form>
  </aside>;
}

function formatCPU(millis: number): string { return `${Number((millis / 1000).toFixed(1))} CPU`; }
function formatMemory(megabytes: number): string { return megabytes >= 1024 ? `${Number((megabytes / 1024).toFixed(1))} GB` : `${megabytes} MB`; }
