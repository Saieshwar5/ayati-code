import type { FormEvent } from "react";
import { useEffect, useState } from "react";
import type {
  EnvironmentVariable,
  Workspace,
  WorkspaceSession,
} from "../api/contracts";
import { api } from "../api/client";

interface EnvironmentPanelProps {
  workspace: Workspace;
  sessions: WorkspaceSession[];
}

export function EnvironmentPanel({ workspace, sessions }: EnvironmentPanelProps) {
  const [variables, setVariables] = useState<EnvironmentVariable[]>([]);
  const [name, setName] = useState("");
  const [value, setValue] = useState("");
  const [exposeDuringSetup, setExposeDuringSetup] = useState(false);
  const [editingName, setEditingName] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const working = sessions.some((session) => session.status === "working");
  const initializing = workspace.status === "creating" || workspace.status === "initializing";
  const locked = working || initializing;

  useEffect(() => {
    resetForm();
    setVariables([]);
    setLoading(true);
    let current = true;
    api.environment(workspace.id).then(
      (items) => {
        if (current) setVariables(items);
        if (current) setLoading(false);
      },
      (reason: Error) => {
        if (current) setError(reason.message);
        if (current) setLoading(false);
      },
    );
    return () => {
      current = false;
    };
  }, [workspace.id]);

  function resetForm() {
    setName("");
    setValue("");
    setExposeDuringSetup(false);
    setEditingName("");
    setError("");
  }

  function edit(variable: EnvironmentVariable) {
    setName(variable.name);
    setValue("");
    setExposeDuringSetup(variable.expose_during_setup);
    setEditingName(variable.name);
    setError("");
  }

  async function reload() {
    setVariables(await api.environment(workspace.id));
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    setSaving(true);
    try {
      await api.upsertEnvironment(workspace.id, {
        name,
        value,
        expose_during_setup: exposeDuringSetup,
      });
      resetForm();
      await reload();
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setSaving(false);
    }
  }

  async function remove(variableName: string) {
    if (!window.confirm(`Delete ${variableName} from this workspace?`)) return;
    setError("");
    try {
      await api.deleteEnvironment(workspace.id, variableName);
      resetForm();
      await reload();
    } catch (reason) {
      setError((reason as Error).message);
    }
  }

  return (
    <section className="inspector-panel active" role="tabpanel">
      <div className="section-heading">
        <div>
          <p className="eyebrow">Development</p>
          <h3>Environment variables</h3>
        </div>
      </div>
      <p className="scope-note">
        Values are write-only, encrypted locally, and available to shell commands in every session.
      </p>
      {locked && (
        <p className="environment-lock-note">
          Variables cannot change {working ? "while an agent is working" : "during initialization"}.
        </p>
      )}
      <div className="environment-list">
        {loading ? (
          <p className="environment-empty">Loading workspace variables…</p>
        ) : variables.length ? (
          variables.map((variable) => (
            <article className="environment-variable" key={variable.name}>
              <div>
                <code>{variable.name}</code>
                <span>{variable.expose_during_setup ? "Setup + development" : "Development"}</span>
              </div>
              <div>
                <button
                  className="quiet compact"
                  type="button"
                  disabled={locked}
                  onClick={() => edit(variable)}
                >
                  Replace
                </button>
                <button
                  className="quiet compact danger-text"
                  type="button"
                  disabled={locked}
                  onClick={() => void remove(variable.name)}
                >
                  Delete
                </button>
              </div>
            </article>
          ))
        ) : (
          <p className="environment-empty">No workspace variables configured.</p>
        )}
      </div>
      <form className="environment-form" onSubmit={(event) => void submit(event)}>
        <label>
          Name
          <input
            value={name}
            disabled={locked || Boolean(editingName)}
            autoComplete="off"
            placeholder="DATABASE_URL"
            required
            onChange={(event) => setName(event.target.value)}
          />
        </label>
        <label>
          Value
          <input
            value={value}
            disabled={locked}
            type="password"
            autoComplete="new-password"
            placeholder="Enter a new value"
            required
            onChange={(event) => setValue(event.target.value)}
          />
        </label>
        <label className="check-row">
          <input
            type="checkbox"
            checked={exposeDuringSetup}
            disabled={locked}
            onChange={(event) => setExposeDuringSetup(event.target.checked)}
          />
          Also expose during dependency setup
        </label>
        {error && <div className="error">{error}</div>}
        <div className="environment-actions">
          {editingName && (
            <button className="quiet" type="button" disabled={locked} onClick={resetForm}>
              Cancel
            </button>
          )}
          <button className="primary" type="submit" disabled={locked || saving}>
            {editingName ? "Replace value" : "Add variable"}
          </button>
        </div>
      </form>
    </section>
  );
}
