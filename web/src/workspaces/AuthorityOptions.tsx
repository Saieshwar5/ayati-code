import type { WorkspaceAuthority } from "../api/contracts";

interface AuthorityOptionsProps {
  value: WorkspaceAuthority;
  onChange: (value: WorkspaceAuthority) => void;
}

export function AuthorityOptions({ value, onChange }: AuthorityOptionsProps) {
  return (
    <fieldset className="authority-options">
      <legend>Authority</legend>
      <label className={`authority-option${value === "explore" ? " selected" : ""}`}>
        <input
          type="radio"
          name="authority"
          value="explore"
          aria-label="Explore authority"
          checked={value === "explore"}
          onChange={() => onChange("explore")}
        />
        <span>
          <strong>Explore</strong>
          <small>Inspect, test and understand. Project files are protected.</small>
        </span>
        <em>Recommended</em>
      </label>
      <label className={`authority-option${value === "develop" ? " selected" : ""}`}>
        <input
          type="radio"
          name="authority"
          value="develop"
          aria-label="Develop authority"
          checked={value === "develop"}
          onChange={() => onChange("develop")}
        />
        <span>
          <strong>Develop</strong>
          <small>Everything in Explore, plus permission to change project files.</small>
        </span>
      </label>
    </fieldset>
  );
}
