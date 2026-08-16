import type { FormEvent } from "react";
import { useEffect, useState } from "react";
import type {
  Branch,
  CreateWorkspaceInput,
  EnvironmentInput,
  Repository,
} from "../api/contracts";
import { api } from "../api/client";
import { CreationEnvironment } from "./CreationEnvironment";

interface WorkspaceHomeProps {
  view: "empty" | "create";
  repositories: Repository[];
  repositoryError: string;
  onShowCreate: () => void;
  onCancel: () => void;
  onCreate: (input: CreateWorkspaceInput) => Promise<void>;
}

export function WorkspaceHome(props: WorkspaceHomeProps) {
  if (props.view === "create") return <CreateWorkspaceForm {...props} />;
  return (
    <section className="workspace-home">
      <div className="workspace-empty">
        <p className="eyebrow">Coding workspace</p>
        <h1>Select a workspace</h1>
        <p className="muted">
          {props.repositoryError
            ? `${props.repositoryError}. Check the GitHub App installation, then reload Ayati.`
            : "Choose an existing project from the left, or create a workspace to prepare a repository and its sandbox."}
        </p>
        <button className="primary" type="button" onClick={props.onShowCreate}>
          Create workspace
        </button>
      </div>
    </section>
  );
}

function CreateWorkspaceForm(props: WorkspaceHomeProps) {
  const [repository, setRepository] = useState("");
  const [branches, setBranches] = useState<Branch[]>([]);
  const [baseBranch, setBaseBranch] = useState("");
  const [branch, setBranch] = useState("");
  const [existingBranch, setExistingBranch] = useState("");
  const [createBranch, setCreateBranch] = useState(true);
  const [setup, setSetup] = useState("");
  const [environment, setEnvironment] = useState<EnvironmentInput[]>([]);
  const [branchesLoading, setBranchesLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState(props.repositoryError);

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
        setBaseBranch(
          props.repositories.find((item) => item.full_name === repository)?.default_branch || "",
        );
        setExistingBranch("");
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
      await props.onCreate({
        repository,
        base_branch: baseBranch,
        branch: createBranch ? branch : existingBranch,
        create_branch: createBranch,
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
    <section className="workspace-home">
      <form className="create-panel" onSubmit={(event) => void submit(event)}>
        <div className="pane-heading">
          <div>
            <p className="eyebrow">New workspace</p>
            <h1>Prepare a project</h1>
            <p className="muted">Choose the repository and working branch Ayati should keep together.</p>
          </div>
          <button className="quiet" type="button" onClick={props.onCancel}>
            Cancel
          </button>
        </div>
        <label>
          Repository
          <select
            value={repository}
            required
            disabled={!props.repositories.length}
            onChange={(event) => setRepository(event.target.value)}
          >
            <option value="">
              {props.repositories.length ? "Select a repository" : "No installed repositories"}
            </option>
            {props.repositories.map((item) => (
              <option key={item.id} value={item.full_name}>
                {item.full_name}
              </option>
            ))}
          </select>
        </label>
        <div className="form-grid">
          <label>
            Base branch
            <select
              value={baseBranch}
              required
              disabled={!repository || branchesLoading}
              onChange={(event) => setBaseBranch(event.target.value)}
            >
              <option value="">{branchesLoading ? "Loading branches…" : "Select a branch"}</option>
              {branches.map((item) => (
                <option key={item.name} value={item.name}>
                  {item.name}
                </option>
              ))}
            </select>
          </label>
          {createBranch ? (
            <label>
              New working branch
              <input
                value={branch}
                placeholder="ayati/my-change"
                required
                onChange={(event) => setBranch(event.target.value)}
              />
            </label>
          ) : (
            <label>
              Working branch
              <select
                value={existingBranch}
                required
                onChange={(event) => setExistingBranch(event.target.value)}
              >
                <option value="">Select a branch</option>
                {branches.map((item) => (
                  <option key={item.name} value={item.name}>
                    {item.name}
                  </option>
                ))}
              </select>
            </label>
          )}
        </div>
        <label className="check-row">
          <input
            type="checkbox"
            checked={createBranch}
            onChange={(event) => setCreateBranch(event.target.checked)}
          />
          Create a new branch from the selected base
        </label>
        <label>
          Setup command <span className="optional">optional, detected automatically</span>
          <input value={setup} placeholder="go mod download" onChange={(event) => setSetup(event.target.value)} />
        </label>
        <CreationEnvironment values={environment} onChange={setEnvironment} />
        {error && (
          <div className="error" role="alert">
            {error}
          </div>
        )}
        <div className="form-actions">
          <button className="primary" type="submit" disabled={submitting}>
            Create and initialize
          </button>
        </div>
      </form>
    </section>
  );
}
