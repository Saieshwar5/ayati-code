import type { EnvironmentInput } from "../api/contracts";

interface CreationEnvironmentProps {
  values: EnvironmentInput[];
  onChange: (values: EnvironmentInput[]) => void;
}

export function CreationEnvironment({ values, onChange }: CreationEnvironmentProps) {
  function update(index: number, change: Partial<EnvironmentInput>) {
    onChange(values.map((value, current) => (current === index ? { ...value, ...change } : value)));
  }

  function remove(index: number) {
    onChange(values.filter((_, current) => current !== index));
  }

  return (
    <details className="creation-disclosure create-environment">
      <summary>
        <span><strong>Environment variables</strong><small>Secrets and configuration for the workspace</small></span>
        <em>{values.length ? `${values.length} added` : "Optional"}</em>
      </summary>
      <div className="disclosure-content">
        <div className="environment-toolbar">
          <p>Encrypted locally and never added to the repository.</p>
          <button
            className="quiet compact"
            type="button"
            onClick={() => onChange([...values, { name: "", value: "", expose_during_setup: false }])}
          >
            Add variable
          </button>
        </div>
        {!values.length && <p className="environment-draft-empty">No workspace variables configured.</p>}
        <div className="environment-drafts">
          {values.map((value, index) => (
            <div className="environment-draft" key={index}>
              <label>
                Name
                <input value={value.name} autoComplete="off" placeholder="DATABASE_URL" required onChange={(event) => update(index, { name: event.target.value })} />
              </label>
              <label>
                Value
                <input value={value.value} type="password" autoComplete="new-password" placeholder="Secret or configuration" required onChange={(event) => update(index, { value: event.target.value })} />
              </label>
              <label className="check-row environment-draft-setup">
                <input type="checkbox" checked={value.expose_during_setup} onChange={(event) => update(index, { expose_during_setup: event.target.checked })} />
                During setup
              </label>
              <button className="quiet compact" type="button" aria-label={`Remove ${value.name || "variable"}`} onClick={() => remove(index)}>
                Remove
              </button>
            </div>
          ))}
        </div>
      </div>
    </details>
  );
}
