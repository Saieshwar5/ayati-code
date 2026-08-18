import { useState } from "react";
import type { ComputeEnvironment } from "../api/environment-contracts";

interface EnvironmentDeleteDialogProps {
  environment: ComputeEnvironment;
  onCancel: () => void;
  onConfirm: () => Promise<void>;
}

export function EnvironmentDeleteDialog(props: EnvironmentDeleteDialogProps) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function confirm() {
    setBusy(true);
    setError("");
    try { await props.onConfirm(); }
    catch (reason) { setError((reason as Error).message); setBusy(false); }
  }

  return <div className="environment-dialog-backdrop" onKeyDown={(event) => {
    if (!busy && event.key === "Escape") props.onCancel();
  }} onMouseDown={(event) => {
    if (!busy && event.target === event.currentTarget) props.onCancel();
  }}>
    <section className="environment-delete-dialog" role="alertdialog" aria-modal="true" aria-labelledby="delete-environment-title">
      <p className="eyebrow">Remove capacity</p>
      <h2 id="delete-environment-title">Delete {props.environment.name}?</h2>
      <p>Only the reusable compute configuration will be removed. This cannot be undone.</p>
      {error && <div className="error" role="alert">{error}</div>}
      <footer><button className="quiet" type="button" autoFocus disabled={busy} onClick={props.onCancel}>Cancel</button><button className="primary" type="button" disabled={busy} onClick={() => void confirm()}>{busy ? "Deleting…" : "Delete environment"}</button></footer>
    </section>
  </div>;
}
