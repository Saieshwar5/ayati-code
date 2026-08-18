import type { WorkspaceAuthority } from "../api/contracts";

interface AuthorityOptionsProps {
  value: WorkspaceAuthority;
  onChange: (value: WorkspaceAuthority) => void;
}

export function AuthorityOptions({ value, onChange }: AuthorityOptionsProps) {
  return (
    <section className="composer-setting" aria-labelledby="authority-title">
      <div className="composer-setting-label">
        <h3 id="authority-title">Access</h3>
        <p>{value === "explore" ? "Project files stay read only." : "Read and change project files."}</p>
      </div>
      <fieldset className="authority-options">
        <legend className="sr-only">Workspace access</legend>
        <AuthorityOption
          value="explore"
          selected={value}
          title="Explore"
          onChange={onChange}
        />
        <AuthorityOption
          value="develop"
          selected={value}
          title="Develop"
          onChange={onChange}
        />
      </fieldset>
    </section>
  );
}

function AuthorityOption(props: {
  value: WorkspaceAuthority;
  selected: WorkspaceAuthority;
  title: string;
  onChange: (value: WorkspaceAuthority) => void;
}) {
  return (
    <label className={props.selected === props.value ? "selected" : ""}>
      <input
        type="radio"
        name="authority"
        value={props.value}
        aria-label={`${props.title} authority`}
        checked={props.selected === props.value}
        onChange={() => props.onChange(props.value)}
      />
      <span>{props.title}</span>
    </label>
  );
}
