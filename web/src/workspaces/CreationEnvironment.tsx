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
    <section className="create-environment" aria-labelledby="create-environment-title">
      <div className="section-heading">
        <div>
          <p className="eyebrow">Environment</p>
          <h3 id="create-environment-title">Workspace variables</h3>
        </div>
        <button
          className="quiet compact"
          type="button"
          onClick={() =>
            onChange([...values, { name: "", value: "", expose_during_setup: false }])
          }
        >
          Add variable
        </button>
      </div>
      <p className="scope-note">
        Encrypted locally and shared by every session. Values are not added to the repository.
      </p>
      <div className="environment-drafts">
        {values.map((value, index) => (
          <div className="environment-draft" key={index}>
            <label>
              Name
              <input
                value={value.name}
                autoComplete="off"
                placeholder="DATABASE_URL"
                required
                onChange={(event) => update(index, { name: event.target.value })}
              />
            </label>
            <label>
              Value
              <input
                value={value.value}
                type="password"
                autoComplete="new-password"
                placeholder="Secret or configuration"
                required
                onChange={(event) => update(index, { value: event.target.value })}
              />
            </label>
            <label className="check-row environment-draft-setup">
              <input
                type="checkbox"
                checked={value.expose_during_setup}
                onChange={(event) => update(index, { expose_during_setup: event.target.checked })}
              />
              During setup
            </label>
            <button
              className="quiet compact"
              type="button"
              aria-label={`Remove ${value.name || "variable"}`}
              onClick={() => remove(index)}
            >
              Remove
            </button>
          </div>
        ))}
      </div>
    </section>
  );
}
