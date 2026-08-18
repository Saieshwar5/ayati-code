import type { Branch } from "../api/contracts";

export type CreationBranchMode = "new" | "existing";

interface BranchSelectionProps {
  mode: CreationBranchMode;
  branches: Branch[];
  defaultBranch: string;
  baseBranch: string;
  newBranch: string;
  existingBranch: string;
  loading: boolean;
  repository: string;
  onModeChange: (mode: CreationBranchMode) => void;
  onBaseChange: (value: string) => void;
  onNewChange: (value: string) => void;
  onExistingChange: (value: string) => void;
}

export function BranchSelection(props: BranchSelectionProps) {
  const complete = props.mode === "new"
    ? Boolean(props.baseBranch && props.newBranch)
    : Boolean(props.existingBranch);
  return (
    <section className="composer-setting branch-selection" aria-labelledby="branch-workflow-title">
      <div className="composer-setting-label">
        <h3 id="branch-workflow-title">Branch</h3>
        <p>{complete ? "Working branch selected." : "Choose where development starts."}</p>
      </div>
      <div className="composer-setting-control branch-control">
        <fieldset className="branch-mode-options">
          <legend className="sr-only">Branch choice</legend>
          <BranchModeOption mode="new" selected={props.mode} title="Create new branch" description="Recommended" onChange={props.onModeChange} />
          <BranchModeOption mode="existing" selected={props.mode} title="Use existing branch" description="Continue work" onChange={props.onModeChange} />
        </fieldset>
        <div className="branch-fields">
          {props.mode === "new" && (
            <div className="form-grid">
              <BranchSelect label="Base branch" value={props.baseBranch} branches={props.branches} defaultBranch={props.defaultBranch} loading={props.loading} repository={props.repository} onChange={props.onBaseChange} />
              <label>New branch name<input value={props.newBranch} placeholder="perpetual/my-change" required onChange={(event) => props.onNewChange(event.target.value)} /></label>
            </div>
          )}
          {props.mode === "existing" && (
            <>
              <BranchSelect label="Working branch" value={props.existingBranch} branches={props.branches} defaultBranch={props.defaultBranch} loading={props.loading} repository={props.repository} onChange={props.onExistingChange} />
              {props.existingBranch && props.existingBranch === props.defaultBranch ? (
                <p className="inline-notice">This works directly on the default branch, so a pull request cannot be created.</p>
              ) : props.existingBranch ? (
                <p className="branch-target-note">Pull requests will target <code>{props.defaultBranch}</code>.</p>
              ) : null}
            </>
          )}
        </div>
      </div>
    </section>
  );
}

function BranchModeOption(props: { mode: CreationBranchMode; selected: CreationBranchMode; title: string; description: string; onChange: (mode: CreationBranchMode) => void }) {
  return (
    <label className={props.selected === props.mode ? "selected" : ""}>
      <input type="radio" name="branch-mode" value={props.mode} aria-label={props.title} checked={props.selected === props.mode} onChange={() => props.onChange(props.mode)} />
      <span><strong>{props.title}</strong><small>{props.description}</small></span>
    </label>
  );
}

export function BranchSelect(props: { label: string; value: string; branches: Branch[]; defaultBranch?: string; loading: boolean; repository: string; onChange: (value: string) => void }) {
  return (
    <label>{props.label}<select value={props.value} required disabled={!props.repository || props.loading} onChange={(event) => props.onChange(event.target.value)}>
      <option value="">{props.loading ? "Loading branches…" : props.repository ? "Select a branch" : "Select a repository first"}</option>
      {props.branches.map((item) => (
        <option key={item.name} value={item.name}>
          {item.name}{item.name === props.defaultBranch ? " · Default" : ""}
        </option>
      ))}
    </select></label>
  );
}
