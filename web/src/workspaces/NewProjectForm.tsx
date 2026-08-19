import type { FormEvent } from "react";
import { useState } from "react";
import type {
  CreateNewProjectInput,
  EnvironmentInput,
  WorkspaceAuthority,
} from "../api/contracts";
import { AuthorityOptions } from "./AuthorityOptions";
import { CreationEnvironment } from "./CreationEnvironment";

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

  return (
    <form className="create-fields" onSubmit={(event) => void submit(event)}>
      <label>Repository name<input value={name} required maxLength={100} placeholder="my-project" onChange={(event) => setName(event.target.value)} /></label>
      <label>Description <span className="optional">optional</span><input value={description} maxLength={350} placeholder="What are you building?" onChange={(event) => setDescription(event.target.value)} /></label>
      <fieldset className="visibility-options">
        <legend>Visibility</legend>
        <label><input type="radio" name="visibility" checked={isPrivate} onChange={() => setPrivate(true)} />Private</label>
        <label><input type="radio" name="visibility" checked={!isPrivate} onChange={() => setPrivate(false)} />Public</label>
      </fieldset>
      <AuthorityOptions value={authority} onChange={setAuthority} />
      {authority === "develop" && (
        <>
          <div className="develop-warning">Develop allows the agent to change project files. Publishing remains an explicit user action.</div>
          <label>New working branch<input value={branch} required placeholder="perpetual/initial" onChange={(event) => setBranch(event.target.value)} /></label>
        </>
      )}
      <label>Setup command <span className="optional">optional, detected automatically</span><input value={setup} placeholder="npm install" onChange={(event) => setSetup(event.target.value)} /></label>
      <CreationEnvironment values={environment} onChange={setEnvironment} />
      {error && <div className="error" role="alert">{error}</div>}
      <p className="scope-note">Perpetual creates the GitHub repository with a README, then uses the normal workspace preparation pipeline.</p>
      <div className="form-actions"><button className="primary" type="submit" disabled={submitting}>{submitting ? "Creating…" : "Create and prepare"}</button></div>
    </form>
  );
}
