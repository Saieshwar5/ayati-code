import type { EnvironmentInput } from "../api/contracts";
import { CreationEnvironment } from "./CreationEnvironment";

interface WorkspaceSetupOptionsProps {
  setup: string;
  environment: EnvironmentInput[];
  setupPlaceholder: string;
  onSetupChange: (value: string) => void;
  onEnvironmentChange: (values: EnvironmentInput[]) => void;
}

export function WorkspaceSetupOptions(props: WorkspaceSetupOptionsProps) {
  const configured = Number(Boolean(props.setup.trim())) + props.environment.length;
  return (
    <section className="composer-setting setup-setting" aria-labelledby="setup-options-title">
      <div className="composer-setting-label">
        <h3 id="setup-options-title">Setup</h3>
        <p>{configured ? `${configured} custom option${configured === 1 ? "" : "s"}.` : "Automatic by default."}</p>
      </div>
      <div className="composer-setting-control setup-options">
        <details className="creation-disclosure">
          <summary>
            <span><strong>Setup command</strong><small>Override automatic detection</small></span>
            <em>{props.setup ? "Added" : "Optional"}</em>
          </summary>
          <div className="disclosure-content">
            <label>Command<input value={props.setup} placeholder={props.setupPlaceholder} onChange={(event) => props.onSetupChange(event.target.value)} /></label>
          </div>
        </details>
        <CreationEnvironment values={props.environment} onChange={props.onEnvironmentChange} />
      </div>
    </section>
  );
}
