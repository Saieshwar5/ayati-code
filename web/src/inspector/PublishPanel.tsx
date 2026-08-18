import type { FormEvent } from "react";
import { useEffect, useState } from "react";
import type { PublishInput, Workspace } from "../api/contracts";

interface PublishPanelProps {
  workspace: Workspace;
  publishing: boolean;
  onPublish: (input: PublishInput) => Promise<boolean>;
}

export function PublishPanel({ workspace, publishing, onPublish }: PublishPanelProps) {
  const [commitMessage, setCommitMessage] = useState("feat: update project");
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    setTitle(workspace.branch.replaceAll("-", " ").replace(/^ayati\//, ""));
    setError("");
  }, [workspace.id, workspace.branch]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    try {
      await onPublish({ commit_message: commitMessage, title, body });
    } catch (reason) {
      setError((reason as Error).message);
    }
  }

  if (workspace.authority === "explore") {
    return (
      <section className="inspector-panel active" role="tabpanel">
        <div className="section-heading">
          <div>
            <p className="eyebrow">GitHub</p>
            <h3>Publishing protected</h3>
          </div>
        </div>
        <p className="scope-note">
          Explore workspaces cannot commit, push or open pull requests. Switch the workspace to
          Develop before publishing changes.
        </p>
      </section>
    );
  }

  if (workspace.branch === workspace.base_branch) {
    return (
      <section className="inspector-panel active" role="tabpanel">
        <div className="section-heading">
          <div>
            <p className="eyebrow">GitHub</p>
            <h3>Pull request unavailable</h3>
          </div>
        </div>
        <p className="scope-note">
          This workspace is working directly on <code>{workspace.branch}</code>. GitHub cannot
          create a pull request when its source and target are the same branch, and perpetual will
          never silently push directly to that branch.
        </p>
      </section>
    );
  }

  return (
    <section className="inspector-panel active" role="tabpanel">
      <div className="section-heading">
        <div>
          <p className="eyebrow">GitHub</p>
          <h3>Publish changes</h3>
        </div>
      </div>
      <p className="scope-note">
        Publishing includes all current workspace changes, including work from other conversations.
      </p>
      {workspace.pull_request_url && (
        <a className="pull-link" href={workspace.pull_request_url} target="_blank" rel="noreferrer">
          Open pull request #{workspace.pull_request_number}
        </a>
      )}
      <form className="publish-form" onSubmit={(event) => void submit(event)}>
        <label>
          Commit message
          <input value={commitMessage} required onChange={(event) => setCommitMessage(event.target.value)} />
        </label>
        <label>
          Pull request title
          <input value={title} required onChange={(event) => setTitle(event.target.value)} />
        </label>
        <label>
          Pull request description
          <textarea
            value={body}
            rows={6}
            placeholder="What changed and how was it verified?"
            onChange={(event) => setBody(event.target.value)}
          />
        </label>
        {error && <div className="error">{error}</div>}
        <button className="primary" type="submit" disabled={publishing}>
          {workspace.pull_request_url ? "Push new changes" : "Create pull request"}
        </button>
      </form>
    </section>
  );
}
