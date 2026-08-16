import type { FormEvent } from "react";
import { useEffect, useState } from "react";
import type {
  Branch,
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
  const [branch, setBranch] = useState("");
  const [existingBranch, setExistingBranch] = useState("");
  const [createBranch, setCreateBranch] = useState(true);
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
      return;
    }
    let current = true;
    setBranchesLoading(true);
    setError("");
    api.branches(repository).then(
      (values) => {
        if (!current) return;
        setBranches(values);
        setBaseBranch(props.repositories.find((item) => item.full_name === repository)?.default_branch || "");
        setExistingBranch("");
        setBranchesLoading(false);
      },
      (reason: Error) => {
        if (!current) return;
        setError(reason.message);
        setBranchesLoading(false);
      },
    );
    return () => { current = false; };
  }, [props.repositories, repository]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setSubmitting(true);
    try {
      await props.onCreate({
        repository,
        base_branch: baseBranch,
        branch: authority === "explore" ? baseBranch : createBranch ? branch : existingBranch,
        create_branch: authority === "develop" && createBranch,
        authority,
        setup_command: setup,
        environment,
      });
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setSubmitting(false);
    }
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
      <div className={authority === "develop" ? "form-grid" : ""}>
        <label>
          Starting branch
          <select value={baseBranch} required disabled={!repository || branchesLoading} onChange={(event) => setBaseBranch(event.target.value)}>
            <option value="">{branchesLoading ? "Loading branches…" : "Select a branch"}</option>
            {branches.map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}
          </select>
        </label>
        {authority === "develop" && (createBranch ? (
          <label>New working branch<input value={branch} placeholder="ayati/my-change" required onChange={(event) => setBranch(event.target.value)} /></label>
        ) : (
          <label>Working branch<select value={existingBranch} required onChange={(event) => setExistingBranch(event.target.value)}>
            <option value="">Select a branch</option>
            {branches.map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}
          </select></label>
        ))}
      </div>
      {authority === "develop" && (
        <label className="check-row"><input type="checkbox" checked={createBranch} onChange={(event) => setCreateBranch(event.target.checked)} />Create a new branch from the selected base</label>
      )}
      <label>Setup command <span className="optional">optional, detected automatically</span><input value={setup} placeholder="go mod download" onChange={(event) => setSetup(event.target.value)} /></label>
      <CreationEnvironment values={environment} onChange={setEnvironment} />
      {error && (
        <div className="error github-reconnect" role="alert">
          <span>{error}</span>
          {props.repositoryReconnectRequired && (
            <a className="button" href="/auth/github">Reconnect GitHub</a>
          )}
        </div>
      )}
      <div className="form-actions"><button className="primary" type="submit" disabled={submitting}>Create and initialize</button></div>
    </form>
  );
}
