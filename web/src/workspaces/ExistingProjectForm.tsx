import type { FormEvent } from "react";
import { useEffect, useState } from "react";
import type {
  Branch,
  BranchMode,
  CreateWorkspaceInput,
  EnvironmentInput,
  Repository,
  WorkspaceAuthority,
} from "../api/contracts";
import { api } from "../api/client";
import { AuthorityOptions } from "./AuthorityOptions";
import { CreationEnvironment } from "./CreationEnvironment";

interface ExistingProjectFormProps {
  repositories: Repository[];
  repositoryError: string;
  repositoryReconnectRequired: boolean;
  onCreate: (input: CreateWorkspaceInput) => Promise<void>;
}

export function ExistingProjectForm(props: ExistingProjectFormProps) {
  const [repository, setRepository] = useState("");
  const [branches, setBranches] = useState<Branch[]>([]);
  const [baseBranch, setBaseBranch] = useState("");
  const [newBranch, setNewBranch] = useState("");
  const [existingBranch, setExistingBranch] = useState("");
  const [directBranch, setDirectBranch] = useState("");
  const [branchMode, setBranchMode] = useState<BranchMode>("new");
  const [authority, setAuthority] = useState<WorkspaceAuthority>("explore");
  const [setup, setSetup] = useState("");
  const [environment, setEnvironment] = useState<EnvironmentInput[]>([]);
  const [branchesLoading, setBranchesLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState(props.repositoryError);

  useEffect(() => setError(props.repositoryError), [props.repositoryError]);

  useEffect(() => {
    if (!repository) {
      setBranches([]);
      setBaseBranch("");
      setExistingBranch("");
      setDirectBranch("");
      return;
    }
    let current = true;
    setBranchesLoading(true);
    setError("");
    api.branches(repository).then(
      (values) => {
        if (!current) return;
        const defaultBranch = props.repositories.find(
          (item) => item.full_name === repository,
        )?.default_branch || values[0]?.name || "";
        setBranches(values);
        setBaseBranch(defaultBranch);
        setExistingBranch("");
        setDirectBranch(defaultBranch);
        setBranchesLoading(false);
      },
      (reason: Error) => {
        if (!current) return;
        setError(reason.message);
        setBranchesLoading(false);
      },
    );
    return () => {
      current = false;
    };
  }, [props.repositories, repository]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await props.onCreate(createInput());
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  function createInput(): CreateWorkspaceInput {
    if (authority === "explore") {
      return {
        repository, base_branch: baseBranch, branch: baseBranch,
        create_branch: false, branch_mode: "direct", authority, setup_command: setup, environment,
      };
    }
    if (branchMode === "new") {
      return {
        repository, base_branch: baseBranch, branch: newBranch,
        create_branch: true, branch_mode: branchMode, authority, setup_command: setup, environment,
      };
    }
    if (branchMode === "existing") {
      return {
        repository, base_branch: baseBranch, branch: existingBranch,
        create_branch: false, branch_mode: branchMode, authority, setup_command: setup, environment,
      };
    }
    return {
      repository, base_branch: directBranch, branch: directBranch,
      create_branch: false, branch_mode: branchMode, authority, setup_command: setup, environment,
    };
  }

  return (
    <form className="create-fields" onSubmit={(event) => void submit(event)}>
      <label>
        Repository
        <select value={repository} required disabled={!props.repositories.length} onChange={(event) => setRepository(event.target.value)}>
          <option value="">{props.repositories.length ? "Select a repository" : "No installed repositories"}</option>
          {props.repositories.map((item) => <option key={item.id} value={item.full_name}>{item.full_name}</option>)}
        </select>
      </label>
      <AuthorityOptions value={authority} onChange={setAuthority} />

      {authority === "explore" ? (
        <BranchSelect label="Branch to inspect" value={baseBranch} branches={branches} loading={branchesLoading} repository={repository} onChange={setBaseBranch} />
      ) : (
        <BranchSelection
          mode={branchMode}
          branches={branches}
          baseBranch={baseBranch}
          newBranch={newBranch}
          existingBranch={existingBranch}
          directBranch={directBranch}
          loading={branchesLoading}
          repository={repository}
          onModeChange={setBranchMode}
          onBaseChange={setBaseBranch}
          onNewChange={setNewBranch}
          onExistingChange={setExistingBranch}
          onDirectChange={setDirectBranch}
        />
      )}

      <label>Setup command <span className="optional">optional, detected automatically</span><input value={setup} placeholder="go mod download" onChange={(event) => setSetup(event.target.value)} /></label>
      <CreationEnvironment values={environment} onChange={setEnvironment} />
      {error && (
        <div className="error github-reconnect" role="alert">
          <span>{error}</span>
          {props.repositoryReconnectRequired && <a className="button" href="/auth/github">Reconnect GitHub</a>}
        </div>
      )}
      <div className="form-actions"><button className="primary" type="submit" disabled={submitting}>Create and initialize</button></div>
    </form>
  );
}

interface BranchSelectionProps {
  mode: BranchMode;
  branches: Branch[];
  baseBranch: string;
  newBranch: string;
  existingBranch: string;
  directBranch: string;
  loading: boolean;
  repository: string;
  onModeChange: (mode: BranchMode) => void;
  onBaseChange: (value: string) => void;
  onNewChange: (value: string) => void;
  onExistingChange: (value: string) => void;
  onDirectChange: (value: string) => void;
}

function BranchSelection(props: BranchSelectionProps) {
  return (
    <section className="branch-selection">
      <div><p className="branch-section-title">Branch workflow</p><p className="muted">Choose where work starts and whether Perpetual creates a local branch.</p></div>
      <fieldset className="branch-mode-options">
        <BranchModeOption mode="new" selected={props.mode} title="Create a new branch" description="Start from a base branch and keep the new branch local until publishing." recommended onChange={props.onModeChange} />
        <BranchModeOption mode="existing" selected={props.mode} title="Continue an existing branch" description="Continue a remote feature branch and target another branch with the pull request." onChange={props.onModeChange} />
        <BranchModeOption mode="direct" selected={props.mode} title="Work directly on a branch" description="Make local changes on the selected branch without pull-request publishing." onChange={props.onModeChange} />
      </fieldset>
      {props.mode === "new" && (
        <div className="form-grid">
          <BranchSelect label="Start from / pull request base" value={props.baseBranch} branches={props.branches} loading={props.loading} repository={props.repository} onChange={props.onBaseChange} />
          <label>New working branch<input value={props.newBranch} placeholder="perpetual/my-change" required onChange={(event) => props.onNewChange(event.target.value)} /></label>
        </div>
      )}
      {props.mode === "existing" && (
        <div className="form-grid">
          <BranchSelect label="Working branch" value={props.existingBranch} branches={props.branches} loading={props.loading} repository={props.repository} onChange={props.onExistingChange} />
          <BranchSelect label="Pull request base" value={props.baseBranch} branches={props.branches} loading={props.loading} repository={props.repository} onChange={props.onBaseChange} />
        </div>
      )}
      {props.mode === "direct" && (
        <>
          <BranchSelect label="Working branch" value={props.directBranch} branches={props.branches} loading={props.loading} repository={props.repository} onChange={props.onDirectChange} />
          <p className="direct-branch-warning">Changes stay local. Perpetual will not push or open a pull request when the working branch is also the pull-request base.</p>
        </>
      )}
    </section>
  );
}

function BranchModeOption(props: { mode: BranchMode; selected: BranchMode; title: string; description: string; recommended?: boolean; onChange: (mode: BranchMode) => void }) {
  return (
    <label className={props.selected === props.mode ? "selected" : ""}>
      <input type="radio" name="branch-mode" value={props.mode} checked={props.selected === props.mode} onChange={() => props.onChange(props.mode)} />
      <span><strong>{props.title}</strong><small>{props.description}</small></span>
      {props.recommended && <em>Recommended</em>}
    </label>
  );
}

function BranchSelect(props: { label: string; value: string; branches: Branch[]; loading: boolean; repository: string; onChange: (value: string) => void }) {
  return (
    <label>{props.label}<select value={props.value} required disabled={!props.repository || props.loading} onChange={(event) => props.onChange(event.target.value)}>
      <option value="">{props.loading ? "Loading branches…" : "Select a branch"}</option>
      {props.branches.map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}
    </select></label>
  );
}
