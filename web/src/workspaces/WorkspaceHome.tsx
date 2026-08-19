import { useState } from "react";
import type {
  CreateNewProjectInput,
  CreateWorkspaceInput,
  Repository,
} from "../api/contracts";
import { ExistingProjectForm } from "./ExistingProjectForm";
import { NewProjectForm } from "./NewProjectForm";

interface WorkspaceHomeProps {
  view: "empty" | "create";
  repositories: Repository[];
  recentRepositories?: string[];
  repositoryError: string;
  repositoryReconnectRequired: boolean;
  onShowCreate: () => void;
  onCancel: () => void;
  onCreate: (input: CreateWorkspaceInput) => Promise<void>;
  onCreateProject: (input: CreateNewProjectInput) => Promise<void>;
}

export function WorkspaceHome(props: WorkspaceHomeProps) {
  if (props.view === "create") return <CreateWorkspaceForm {...props} />;
  return (
    <section className="workspace-home">
      <div className="workspace-empty">
        <p className="eyebrow">Coding workspace</p>
        <h1>Select a workspace</h1>
        <p className="muted">
          {props.repositoryReconnectRequired
            ? props.repositoryError
            : props.repositoryError
            ? `${props.repositoryError}. Check the GitHub App installation, then reload perpetual.`
            : "Choose an existing project from the left, or create a workspace to prepare a repository and its sandbox."}
        </p>
        {props.repositoryReconnectRequired ? (
          <a className="primary button" href="/auth/github">Reconnect GitHub</a>
        ) : (
          <button className="primary" type="button" onClick={props.onShowCreate}>
            Create workspace
          </button>
        )}
      </div>
    </section>
  );
}

function CreateWorkspaceForm(props: WorkspaceHomeProps) {
  const [source, setSource] = useState<"existing" | "new">("existing");
  return (
    <section className="workspace-home create-workspace-page">
      <div className="create-panel">
        <header className="create-heading">
          <button className="create-back" type="button" onClick={props.onCancel}>
            <span aria-hidden="true">←</span>
            Workspaces
          </button>
          <div>
            <h1>Create workspace</h1>
            <p className="muted">Choose a project and configure how perpetual should prepare it.</p>
          </div>
        </header>
        <fieldset className="source-options">
          <legend className="sr-only">Project source</legend>
          <label className={source === "existing" ? "selected" : ""}>
            <input
              type="radio"
              name="project-source"
              aria-label="Existing repository"
              checked={source === "existing"}
              onChange={() => setSource("existing")}
            />
            <span><strong>Existing repository</strong></span>
          </label>
          <label className={source === "new" ? "selected" : ""}>
            <input
              type="radio"
              name="project-source"
              aria-label="New project"
              checked={source === "new"}
              onChange={() => setSource("new")}
            />
            <span><strong>New GitHub project</strong></span>
          </label>
        </fieldset>
        {source === "existing" ? (
          <ExistingProjectForm
            repositories={props.repositories}
            recentRepositories={props.recentRepositories}
            repositoryError={props.repositoryError}
            repositoryReconnectRequired={props.repositoryReconnectRequired}
            onCreate={props.onCreate}
          />
        ) : (
          <NewProjectForm onCreate={props.onCreateProject} />
        )}
      </div>
    </section>
  );
}
