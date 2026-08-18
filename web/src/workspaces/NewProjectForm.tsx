import type { FormEvent } from "react";
import { useState } from "react";
import type {
  CreateNewProjectInput,
  EnvironmentInput,
  WorkspaceAuthority,
} from "../api/contracts";
import { AuthorityOptions } from "./AuthorityOptions";
import { WorkspaceCreateAction } from "./WorkspaceSetupSummary";
import { WorkspaceSetupOptions } from "./WorkspaceSetupOptions";

interface NewProjectFormProps {
  onCreate: (input: CreateNewProjectInput) => Promise<void>;
}

export function NewProjectForm({ onCreate }: NewProjectFormProps) {
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [isPrivate, setPrivate] = useState(true);
  const [authority, setAuthority] = useState<WorkspaceAuthority>("explore");
  const [branch, setBranch] = useState("perpetual/initial");
  const [setup, setSetup] = useState("");
  const [environment, setEnvironment] = useState<EnvironmentInput[]>([]);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError("");
    try {
      await onCreate({
        name,
        description,
        private: isPrivate,
        authority,
        branch: authority === "develop" ? branch : "",
        setup_command: setup,
        environment,
      });
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setSubmitting(false);
    }
  }

  const environmentComplete = environment.every((value) => value.name.trim() && value.value);
  const canSubmit = Boolean(name.trim() && (authority === "explore" || branch.trim()) && environmentComplete);

  return (
    <form className="workspace-composer new-project-composer" onSubmit={(event) => void submit(event)}>
      <aside className="project-identity-pane" aria-labelledby="project-details-title">
        <div>
          <p className="eyebrow">GitHub repository</p>
          <h2 id="project-details-title">New project</h2>
          <p>Perpetual creates this repository in your GitHub account.</p>
        </div>
        <label>Repository name<input value={name} required maxLength={100} placeholder="my-project" onChange={(event) => setName(event.target.value)} /></label>
        <label>Description <span className="optional">optional</span><input value={description} maxLength={350} placeholder="What are you building?" onChange={(event) => setDescription(event.target.value)} /></label>
        <fieldset className="visibility-options">
          <legend>Visibility</legend>
          <div>
            <label className={isPrivate ? "selected" : ""}><input type="radio" name="visibility" checked={isPrivate} onChange={() => setPrivate(true)} />Private</label>
            <label className={!isPrivate ? "selected" : ""}><input type="radio" name="visibility" checked={!isPrivate} onChange={() => setPrivate(false)} />Public</label>
          </div>
        </fieldset>
      </aside>
      <section className="composer-settings" aria-label="Workspace settings">
        <header className="composer-project-heading">
          <div>
            <p className="eyebrow">Workspace settings</p>
            <h2>{name || "Untitled project"}</h2>
          </div>
          <span className="repository-visibility">{isPrivate ? "Private" : "Public"}</span>
        </header>
        <AuthorityOptions value={authority} onChange={setAuthority} />

        <section className="composer-setting" aria-labelledby="initial-branch-title">
          <div className="composer-setting-label">
            <h3 id="initial-branch-title">Branch</h3>
            <p>{authority === "develop" ? "Where development begins." : "GitHub default, read only."}</p>
          </div>
          <div className="composer-setting-control">
            {authority === "develop" ? (
              <label>New working branch<input value={branch} required placeholder="perpetual/initial" onChange={(event) => setBranch(event.target.value)} /></label>
            ) : (
              <div className="selection-metadata"><span>Created with a README</span><code>Default branch</code></div>
            )}
          </div>
        </section>

        <WorkspaceSetupOptions setup={setup} environment={environment} setupPlaceholder="npm install" onSetupChange={setSetup} onEnvironmentChange={setEnvironment} />

        {error && <div className="error" role="alert">{error}</div>}
        <WorkspaceCreateAction
          project={name}
          authority={authority}
          branch={authority === "develop" ? branch : "Default branch"}
          environmentCount={environment.length}
          hasSetupCommand={Boolean(setup.trim())}
          canSubmit={canSubmit}
          submitting={submitting}
        />
      </section>
    </form>
  );
}
