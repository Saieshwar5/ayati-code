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
            ? `${props.repositoryError}. Check the GitHub App installation, then reload Ayati.`
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
    <section className="workspace-home">
      <div className="create-panel">
        <div className="pane-heading">
          <div>
            <p className="eyebrow">New workspace</p>
            <h1>Prepare a project</h1>
            <p className="muted">Choose where the project comes from, then let Ayati prepare it.</p>
          </div>
          <button className="quiet" type="button" onClick={props.onCancel}>
            Cancel
          </button>
        </div>
        <fieldset className="source-options">
          <legend>Source</legend>
          <label className={source === "existing" ? "selected" : ""}>
            <input
              type="radio"
              name="project-source"
              aria-label="Existing repository"
              checked={source === "existing"}
              onChange={() => setSource("existing")}
            />
            <span><strong>Existing repository</strong><small>Prepare a repository already connected to Ayati.</small></span>
          </label>
          <label className={source === "new" ? "selected" : ""}>
            <input
              type="radio"
              name="project-source"
              aria-label="New project"
              checked={source === "new"}
              onChange={() => setSource("new")}
            />
            <span><strong>New project</strong><small>Create a GitHub repository and prepare it here.</small></span>
          </label>
        </fieldset>
        {source === "existing" ? (
          <ExistingProjectForm
            repositories={props.repositories}
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
