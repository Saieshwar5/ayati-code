import { useEffect, useState, type ReactNode } from "react";
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

const steps = ["Repository", "Project", "Dependencies", "Verify", "Ready"] as const;

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
      {!props.embedded && (
        <div className="readiness-heading">
          <div>
            <p className="eyebrow">{workspace.repository}</p>
            <h1>{readinessTitle(workspace)}</h1>
            <p className="muted">{workspace.branch}</p>
          </div>
          <span className={`status ${workspace.status}`}>{workspace.status.replaceAll("_", " ")}</span>
        </div>
      )}

      {workspace.status === "deleting" || workspace.status === "deletion_failed" ? (
        <section className="readiness-card readiness-state-card">
          <p className="eyebrow">Local workspace deletion</p>
          <h2>{workspace.status === "deleting" ? "Deleting local workspace…" : "Deletion needs attention"}</h2>
          <p className="muted">{workspace.status === "deleting" ? "Removing the local runtime, clone, cache, and conversation history." : workspace.error || "The local workspace could not be completely removed."}</p>
          <p className="muted">The GitHub repository, remote branches, and pull requests are unchanged.</p>
          {workspace.status === "deletion_failed" && (
            <div className="readiness-actions">
              <button className="quiet danger" type="button" disabled={busy} onClick={() => run(props.onDelete)}>{busy ? "Deleting…" : "Retry deletion"}</button>
            </div>
          )}
        </section>
      ) : workspace.status === "stopped" ? (
        <section className="readiness-card readiness-state-card">
          <p className="eyebrow">Environment stopped</p>
          <h2>Your project and conversations are preserved</h2>
          <p className="muted">Resume to recreate the runtime with the same workspace access.</p>
          <div className="readiness-actions">
            <button className="primary" type="button" disabled={busy} onClick={() => run(props.onResume)}>{busy ? "Resuming…" : "Resume environment"}</button>
          </div>
        </section>
      ) : workspace.status === "needs_configuration" ? (
        <PreparationFrame workspace={workspace} current={1} title="Choose the project to prepare">
          <ProjectSelection
            workspace={workspace}
            projectRoot={projectRoot}
            busy={busy}
            onChange={setProjectRoot}
            onContinue={() => run(() => props.onConfigure(projectRoot))}
          />
        </PreparationFrame>
      ) : workspace.status === "initialization_failed" ? (
        <PreparationFrame workspace={workspace} current={stageIndex(workspace.preparation_failed_stage)} failed title={`${stageLabel(workspace.preparation_failed_stage)} needs attention`}>
          <PreparationFailure workspace={workspace} busy={busy} onRetry={() => run(props.onRetry)} onDelete={() => run(props.onDelete)} />
        </PreparationFrame>
      ) : (
        <PreparationProgress workspace={workspace} />
      )}

      {error && <p className="error readiness-error" role="alert">{error}</p>}
    </section>
  );
}

function PreparationProgress({ workspace }: { workspace: Workspace }) {
  const current = stageIndex(workspace.preparation_stage);
  return (
    <PreparationFrame workspace={workspace} current={current} title={current === 4 ? "Workspace ready" : "Preparing your workspace"}>
      <div className="preparation-current-detail">
        <span className="preparation-pulse" aria-hidden="true" />
        <div>
          <strong>{steps[current] || "Repository"}</strong>
          <p>{workspace.preparation_detail || defaultDetail(current)}</p>
        </div>
      </div>
      <p className="preparation-background-note">Preparation continues if you leave this page.</p>
    </PreparationFrame>
  );
}

function PreparationFrame(props: { workspace: Workspace; current: number; title: string; failed?: boolean; children: ReactNode }) {
  return (
    <section className={`readiness-card preparation-card${props.failed ? " failure" : ""}`}>
      <div className="readiness-card-heading">
        <div>
          <p className="eyebrow">{props.failed ? "Preparation paused" : "Workspace preparation"}</p>
          <h2>{props.title}</h2>
        </div>
        <span className="preparation-project">{repositoryName(props.workspace.repository)}</span>
      </div>
      <ol className="preparation-rail" aria-label="Preparation progress">
        {steps.map((step, index) => {
          const state = index < props.current ? "done" : index === props.current ? (props.failed ? "failed" : "current") : "pending";
          return (
            <li className={state} key={step} aria-current={state === "current" || state === "failed" ? "step" : undefined}>
              <span aria-hidden="true">{state === "done" ? "✓" : index + 1}</span>
              <strong>{step}</strong>
            </li>
          );
        })}
      </ol>
      <div className="preparation-detail-panel">{props.children}</div>
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
    <div className="project-selection">
      <div>
        <strong>Multiple applications were found</strong>
        <p>Choose one project root for this workspace.</p>
      </div>
      <fieldset className="candidate-options">
        <legend className="sr-only">Project root</legend>
        {props.workspace.configuration_candidates.map((candidate) => (
          <label className={`candidate-option${props.projectRoot === candidate.project_root ? " selected" : ""}`} key={candidate.project_root}>
            <input type="radio" name="project-root" value={candidate.project_root} checked={props.projectRoot === candidate.project_root} onChange={() => props.onChange(candidate.project_root)} />
            <span><strong>{candidate.project_root}</strong><small>{candidateSummary(candidate.languages, candidate.package_managers)}</small></span>
          </label>
        ))}
      </fieldset>
      <div className="readiness-actions">
        <button className="primary" type="button" disabled={props.busy || !props.projectRoot} onClick={props.onContinue}>{props.busy ? "Preparing…" : "Continue preparation"}</button>
      </div>
    </div>
  );
}

function PreparationFailure(props: Pick<WorkspaceReadinessProps, "workspace"> & { busy: boolean; onRetry: () => void; onDelete: () => void }) {
  return (
    <div className="preparation-failure">
      <strong>{stageLabel(props.workspace.preparation_failed_stage)} could not finish</strong>
      <p className="failure-message">{props.workspace.error || "Workspace preparation failed."}</p>
      <p className="muted">The repository and conversation history are preserved.</p>
      <div className="readiness-actions">
        <button className="primary" type="button" disabled={props.busy} onClick={props.onRetry}>Retry preparation</button>
        <button className="quiet danger" type="button" disabled={props.busy} onClick={props.onDelete}>Delete workspace…</button>
      </div>
    </div>
  );
}

function readinessTitle(workspace: Workspace): string {
  if (workspace.status === "deleting") return "Deleting local workspace";
  if (workspace.status === "deletion_failed") return "Deletion needs attention";
  if (workspace.status === "needs_configuration") return "Choose a project";
  if (workspace.status === "initialization_failed") return "Preparation needs attention";
  if (workspace.status === "stopped") return `${repositoryName(workspace.repository)} is stopped`;
  return `Preparing ${repositoryName(workspace.repository)}`;
}

function candidateSummary(languages: string[], managers: string[]): string {
  return [...languages, ...managers].join(" · ") || "Generic project";
}

function stageIndex(stage?: PreparationStage): number {
  if (stage === "analyzing" || stage === "needs_configuration") return 1;
  if (stage === "installing") return 2;
  if (stage === "verifying" || stage === "sealing") return 3;
  if (stage === "ready") return 4;
  return 0;
}

function stageLabel(stage?: PreparationStage): string {
  return steps[stageIndex(stage)];
}

function defaultDetail(index: number): string {
  return [
    "Cloning the selected branch.",
    "Detecting the project structure.",
    "Installing project dependencies.",
    "Checking the baseline and applying workspace protection.",
    "The workspace is ready to use.",
  ][index];
}
