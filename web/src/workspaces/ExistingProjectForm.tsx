import type { FormEvent } from "react";
import { useEffect, useState } from "react";
import type {
  Branch,
  CreateWorkspaceInput,
  EnvironmentInput,
  Repository,
} from "../api/contracts";
import { api } from "../api/client";
import { BranchSelection, type CreationBranchMode } from "./BranchSelection";
import { RepositoryPicker } from "./RepositoryPicker";
import { WorkspaceCreateAction } from "./WorkspaceSetupSummary";
import { WorkspaceSetupOptions } from "./WorkspaceSetupOptions";

interface ExistingProjectFormProps {
  repositories: Repository[];
  recentRepositories?: string[];
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
  const [branchMode, setBranchMode] = useState<CreationBranchMode>("new");
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
        const defaultBranch = props.repositories.find(
          (item) => item.full_name === repository,
        )?.default_branch || values[0]?.name || "";
        setBranches(values);
        setBaseBranch(defaultBranch);
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

  const selectedRepository = props.repositories.find((item) => item.full_name === repository);
  const defaultBranch = selectedRepository?.default_branch || baseBranch;

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
    if (branchMode === "new") {
      return {
        repository, base_branch: baseBranch, branch: newBranch,
        create_branch: true, branch_mode: branchMode, setup_command: setup, environment,
      };
    }
    const worksDirectlyOnDefault = existingBranch === defaultBranch;
    return {
      repository,
      base_branch: worksDirectlyOnDefault ? existingBranch : defaultBranch,
      branch: existingBranch,
      create_branch: false,
      branch_mode: worksDirectlyOnDefault ? "direct" : "existing",
      setup_command: setup,
      environment,
    };
  }

  const selectedBranch = branchMode === "new" ? newBranch : existingBranch;
  const environmentComplete = environment.every((value) => value.name.trim() && value.value);
  const canSubmit = Boolean(repository && selectedBranch && environmentComplete);

  return (
    <form className={`workspace-composer${repository ? " repository-chosen" : ""}`} onSubmit={(event) => void submit(event)}>
      <RepositoryPicker repositories={props.repositories} recentRepositories={props.recentRepositories} value={repository} onChange={setRepository} />
      <section className="composer-settings" aria-label="Workspace settings">
        <header className="composer-project-heading">
          <div>
            <p className="eyebrow">Workspace settings</p>
            <h2>{selectedRepository?.full_name || "Select a repository"}</h2>
          </div>
          {selectedRepository && (
            <div className="repository-badges">
              <code>{selectedRepository.default_branch} · Default</code>
              <span>{selectedRepository.private ? "Private" : "Public"}</span>
            </div>
          )}
        </header>
        <BranchSelection
          mode={branchMode}
          branches={branches}
          defaultBranch={defaultBranch}
          baseBranch={baseBranch}
          newBranch={newBranch}
          existingBranch={existingBranch}
          loading={branchesLoading}
          repository={repository}
          onModeChange={(mode) => {
            setBranchMode(mode);
            if (mode === "existing") setBaseBranch(defaultBranch);
          }}
          onBaseChange={setBaseBranch}
          onNewChange={setNewBranch}
          onExistingChange={setExistingBranch}
        />

        <WorkspaceSetupOptions setup={setup} environment={environment} setupPlaceholder="go mod download" onSetupChange={setSetup} onEnvironmentChange={setEnvironment} />

        {error && (
          <div className="error github-reconnect" role="alert">
            <span>{error}</span>
            {props.repositoryReconnectRequired && <a className="button" href="/auth/github">Reconnect GitHub</a>}
          </div>
        )}
        <WorkspaceCreateAction
          project={repository}
          branch={selectedBranch}
          environmentCount={environment.length}
          hasSetupCommand={Boolean(setup.trim())}
          canSubmit={canSubmit}
          submitting={submitting}
        />
      </section>
    </form>
  );
}
