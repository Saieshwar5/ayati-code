import { useEffect, useState } from "react";
import type {
  AuthorityChangeInput,
  Workspace,
  WorkspaceAuthority,
} from "../api/contracts";

interface AuthorityControlProps {
  workspace: Workspace;
  agentWorking: boolean;
  onChange: (input: AuthorityChangeInput) => Promise<void>;
}

export function AuthorityControl(props: AuthorityControlProps) {
  const { workspace } = props;
  const [target, setTarget] = useState<WorkspaceAuthority>();
  const [branch, setBranch] = useState("ayati/change");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const locked = workspace.status !== "ready" || props.agentWorking || busy;

  useEffect(() => {
    setTarget(undefined);
    setBranch(workspace.branch !== workspace.base_branch ? workspace.branch : "ayati/change");
    setError("");
  }, [workspace.id, workspace.authority, workspace.base_branch, workspace.branch]);

  function choose(authority: WorkspaceAuthority) {
    if (authority === workspace.authority || locked) return;
    setTarget(authority);
    setError("");
    if (authority === "develop" && workspace.branch !== workspace.base_branch) {
      setBranch(workspace.branch);
    }
  }

  async function confirm() {
    if (!target) return;
    setBusy(true);
    setError("");
    try {
      await props.onChange({
        authority: target,
        branch: target === "develop" ? branch.trim() : workspace.branch,
        create_branch: target === "develop" && workspace.branch === workspace.base_branch
          ? true
          : workspace.create_branch,
      });
      setTarget(undefined);
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Authority could not be changed.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="authority-control">
      <div className="authority-control-heading">
        <div>
          <p className="profile-section-title">Authority</p>
          <p>{workspace.authority === "explore" ? "Source protected" : "Project editable"}</p>
        </div>
        <span className={`mount-badge ${workspace.effective_mount_mode || "pending"}`}>
          {workspace.effective_mount_mode === "ro" ? "Read only" : workspace.effective_mount_mode === "rw" ? "Read + write" : "Pending"}
        </span>
      </div>
      <fieldset className="authority-toggle">
        <legend className="hidden">Workspace authority</legend>
        <label className={workspace.authority === "explore" ? "selected" : ""}>
          <input
            type="radio"
            name={`authority-${workspace.id}`}
            checked={workspace.authority === "explore"}
            disabled={locked}
            onChange={() => choose("explore")}
          />
          Explore
        </label>
        <label className={workspace.authority === "develop" ? "selected" : ""}>
          <input
            type="radio"
            name={`authority-${workspace.id}`}
            checked={workspace.authority === "develop"}
            disabled={locked}
            onChange={() => choose("develop")}
          />
          Develop
        </label>
      </fieldset>
      {props.agentWorking && <p className="authority-note">Authority cannot change while the agent is working.</p>}
      {workspace.status !== "ready" && <p className="authority-note">The workspace must be ready before authority can change.</p>}

      {target && (
        <div className="authority-confirmation" role="dialog" aria-modal="true" aria-labelledby="authority-confirmation-title">
          <p className="eyebrow">Confirm authority</p>
          <h4 id="authority-confirmation-title">
            {target === "develop" ? "Enable development?" : "Protect project files?"}
          </h4>
          <p className="muted">
            {target === "develop"
              ? "The agent can create, modify and delete project files after an explicit implementation request."
              : "Current modifications are preserved, but the agent cannot change project files."}
          </p>
          {target === "develop" && workspace.branch === workspace.base_branch && (
            <>
              <label>
                Working branch
                <input value={branch} autoFocus onChange={(event) => setBranch(event.target.value)} />
              </label>
              <p className="authority-note">The branch stays local until you publish the workspace.</p>
            </>
          )}
          {target === "develop" && workspace.branch !== workspace.base_branch && (
            <p className="branch-reuse">Continue on <code>{workspace.branch}</code></p>
          )}
          {error && <p className="error" role="alert">{error}</p>}
          <div className="authority-actions">
            <button className="quiet" type="button" disabled={busy} onClick={() => setTarget(undefined)}>Cancel</button>
            <button className="primary" type="button" disabled={busy || (target === "develop" && !branch.trim())} onClick={() => void confirm()}>
              {busy ? "Changing…" : target === "develop" ? "Enable Develop" : "Switch to Explore"}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
