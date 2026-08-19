import { useEffect, useState } from "react";
import type { PreparationStage, Workspace } from "../api/contracts";
import { repositoryName } from "../app/format";

interface WorkspaceReadinessProps {
  workspace: Workspace;
  embedded?: boolean;
  onConfigure: (projectRoot: string) => Promise<void>;
  onRetry: () => Promise<void>;
  onResume: () => Promise<void>;
  onDelete: () => Promise<void>;
}

const steps: Array<{ stage: PreparationStage; label: string }> = [
  { stage: "cloning", label: "Repository cloned" },
  { stage: "analyzing", label: "Project understood" },
  { stage: "installing", label: "Dependencies installed" },
  { stage: "verifying", label: "Baseline verified" },
  { stage: "sealing", label: "Source protection applied" },
  { stage: "ready", label: "Workspace ready" },
];

export function WorkspaceReadiness(props: WorkspaceReadinessProps) {
  const { workspace } = props;
  const [projectRoot, setProjectRoot] = useState(workspace.configuration_candidates[0]?.project_root || "");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    setProjectRoot(workspace.configuration_candidates[0]?.project_root || "");
    setError("");
  }, [workspace.id, workspace.configuration_candidates]);

  async function run(action: () => Promise<void>) {
    setBusy(true);
    setError("");
    try {
      await action();
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "The workspace could not be updated.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className={`readiness-view${props.embedded ? " embedded" : ""}`} aria-live="polite">
      {!props.embedded && <div className="readiness-heading">
        <div>
          <p className="eyebrow">{workspace.repository}</p>
          <h1>{readinessTitle(workspace)}</h1>
          <p className="muted">
            {workspace.branch} · {workspace.authority === "explore" ? "Protected Explore" : "Develop"}
          </p>
        </div>
        <span className={`status ${workspace.status}`}>{workspace.status.replaceAll("_", " ")}</span>
      </div>}

      {workspace.status === "needs_configuration" ? (
        <ProjectSelection
          workspace={workspace}
          projectRoot={projectRoot}
          busy={busy}
          onChange={setProjectRoot}
          onContinue={() => run(() => props.onConfigure(projectRoot))}
        />
      ) : workspace.status === "initialization_failed" ? (
        <PreparationFailure
          workspace={workspace}
          busy={busy}
          onRetry={() => run(props.onRetry)}
          onDelete={() => run(props.onDelete)}
        />
      ) : workspace.status === "stopped" ? (
        <section className="readiness-card">
          <p className="eyebrow">Environment stopped</p>
          <h2>Your project and sessions are preserved</h2>
          <p className="muted">Resume to recreate the sandbox with the same workspace authority.</p>
          <button className="primary" type="button" disabled={busy} onClick={() => run(props.onResume)}>
            {busy ? "Resuming…" : "Resume environment"}
          </button>
        </section>
      ) : (
        <PreparationProgress workspace={workspace} />
      )}

      {error && <p className="error readiness-error" role="alert">{error}</p>}
    </section>
  );
}

function PreparationProgress({ workspace }: { workspace: Workspace }) {
  const current = steps.findIndex((step) => step.stage === workspace.preparation_stage);
  return (
    <section className="readiness-card">
      <div className="readiness-card-heading">
        <div>
          <p className="eyebrow">Preparing workspace</p>
          <h2>Perpetual is making this project ready</h2>
        </div>
        <span className="preparation-pulse" aria-hidden="true" />
      </div>
      <ol className="preparation-list">
        {steps.map((step, index) => {
          const state = current < 0 ? "pending" : index < current ? "done" : index === current ? "current" : "pending";
          return (
            <li className={state} key={step.stage}>
              <span className="step-marker" aria-hidden="true">{state === "done" ? "✓" : index + 1}</span>
              <span>
                <strong>{step.label}</strong>
                {state === "current" && workspace.preparation_detail && <small>{workspace.preparation_detail}</small>}
              </span>
            </li>
          );
        })}
      </ol>
    </section>
  );
}

interface ProjectSelectionProps {
  workspace: Workspace;
  projectRoot: string;
  busy: boolean;
  onChange: (root: string) => void;
  onContinue: () => void;
}

function ProjectSelection(props: ProjectSelectionProps) {
  return (
    <section className="readiness-card">
      <p className="eyebrow">Needs configuration</p>
      <h2>Which project should Perpetual prepare?</h2>
      <p className="muted">Multiple applications were found. Choose one project root for this workspace.</p>
      <fieldset className="candidate-options">
        <legend>Project root</legend>
        {props.workspace.configuration_candidates.map((candidate) => (
          <label className={`candidate-option${props.projectRoot === candidate.project_root ? " selected" : ""}`} key={candidate.project_root}>
            <input
              type="radio"
              name="project-root"
              value={candidate.project_root}
              checked={props.projectRoot === candidate.project_root}
              onChange={() => props.onChange(candidate.project_root)}
            />
            <span>
              <strong>{candidate.project_root}</strong>
              <small>{candidateSummary(candidate.languages, candidate.package_managers)}</small>
            </span>
          </label>
        ))}
      </fieldset>
      <div className="readiness-actions">
        <button className="primary" type="button" disabled={props.busy || !props.projectRoot} onClick={props.onContinue}>
          {props.busy ? "Preparing…" : "Continue preparation"}
        </button>
      </div>
    </section>
  );
}

function PreparationFailure(props: Pick<WorkspaceReadinessProps, "workspace"> & {
  busy: boolean;
  onRetry: () => void;
  onDelete: () => void;
}) {
  return (
    <section className="readiness-card failure">
      <p className="eyebrow">Preparation stopped</p>
      <h2>{failedStep(props.workspace.preparation_failed_stage)} could not finish</h2>
      <p className="failure-message">{props.workspace.error || "Workspace preparation failed."}</p>
      <p className="muted">The repository and session history are preserved. Retry after fixing the reported issue.</p>
      <div className="readiness-actions">
        <button className="primary" type="button" disabled={props.busy} onClick={props.onRetry}>Retry preparation</button>
        <button className="quiet danger" type="button" disabled={props.busy} onClick={props.onDelete}>Delete workspace…</button>
      </div>
    </section>
  );
}

function readinessTitle(workspace: Workspace): string {
  if (workspace.status === "needs_configuration") return "Choose a project";
  if (workspace.status === "initialization_failed") return "Preparation needs attention";
  if (workspace.status === "stopped") return `${repositoryName(workspace.repository)} is stopped`;
  return `Preparing ${repositoryName(workspace.repository)}`;
}

function candidateSummary(languages: string[], managers: string[]): string {
  return [...languages, ...managers].join(" · ") || "Generic project";
}

function failedStep(stage?: PreparationStage): string {
  return steps.find((step) => step.stage === stage)?.label || "Workspace preparation";
}
