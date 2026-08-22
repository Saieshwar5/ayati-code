import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { Run, RunState, RunStep } from "../api/contracts";
import { useRunEvents } from "./useRunEvents";

interface RunTimelineProps {
  workspaceID: string;
  sessionID: string;
}

export function RunTimeline({ workspaceID, sessionID }: RunTimelineProps) {
  const [runs, setRuns] = useState<Run[]>([]);
  const [steps, setSteps] = useState<RunStep[]>([]);
  const [activeRun, setActiveRun] = useState<Run | undefined>();
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const values = await api.runs(workspaceID);
      const scoped = values.filter((run) => run.session_id === sessionID);
      setRuns(scoped);
      const active = scoped.find((run) => run.state === "queued" || run.state === "running" || run.state === "waiting_user")
        || scoped[0];
      setActiveRun(active);
      setError("");
      if (active) {
        const activeSteps = await api.runSteps(active.id);
        setSteps(activeSteps);
      } else {
        setSteps([]);
      }
    } catch (reason) {
      setError((reason as Error).message);
    }
  }, [workspaceID, sessionID]);

  useEffect(() => {
    void load();
  }, [load]);

  useRunEvents(workspaceID, () => void load());

  async function act(action: "stop" | "pause" | "continue") {
    if (!activeRun || busy) return;
    setBusy(true);
    setError("");
    try {
      const updated = await api.runAction(activeRun.id, action);
      setActiveRun(updated);
      setRuns((current) => current.map((run) => (run.id === updated.id ? updated : run)));
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <aside className="run-timeline" aria-label="Execution room timeline">
      <header className="run-timeline-heading">
        <div>
          <p className="eyebrow">Execution room</p>
          <h2>Run timeline</h2>
        </div>
        {activeRun && (
          <RunStatePill state={activeRun.state} />
        )}
      </header>
      {error && (
        <p className="run-timeline-error" role="alert">{error}</p>
      )}
      {!activeRun && !error && (
        <p className="run-timeline-empty">No execution room yet. Send a message to start one.</p>
      )}
      {activeRun && (
        <div className="run-timeline-controls">
          {activeRun.state === "running" || activeRun.state === "queued" ? (
            <>
              <button type="button" disabled={busy} onClick={() => void act("stop")}>Stop</button>
              <button type="button" disabled={busy} onClick={() => void act("pause")}>Pause</button>
            </>
          ) : activeRun.state === "waiting_user" ? (
            <button type="button" disabled={busy} onClick={() => void act("continue")}>Continue</button>
          ) : null}
        </div>
      )}
      {activeRun?.prompt && (
        <div className="run-prompt">User: {activeRun.prompt}</div>
      )}
      {activeRun && (
        <ol className="run-steps" aria-label="Run steps">
          {!steps.length && <li className="run-step run-step-idle">Waiting for the first step…</li>}
          {steps.map((step) => <RunStepView key={step.run_id + ":" + step.step_key} step={step} />)}
        </ol>
      )}
    </aside>
  );
}

function RunStatePill({ state }: { state: RunState }) {
  return <span className={`run-state-pill ${state}`}>{state.replace("_", " ")}</span>;
}

function RunStepView({ step }: { step: RunStep }) {
  if (step.kind === "shell") {
    const command = String(step.input?.command ?? "");
    const stdout = String(step.output?.stdout ?? "");
    const stderr = String(step.output?.stderr ?? "");
    const exitCode = Number(step.output?.exit_code ?? 0);
    return (
      <li className="run-step run-step-shell">
        <div className="run-step-heading"><strong>Shell</strong><span>exit {exitCode}</span></div>
        <pre className="run-step-command">$ {command}</pre>
        {stdout && <pre className="run-step-output">{stdout}</pre>}
        {stderr && <pre className="run-step-output run-step-error">{stderr}</pre>}
      </li>
    );
  }
  if (step.kind === "compact") {
    return <li className="run-step run-step-compact">Context compacted</li>;
  }
  const content = String(step.output?.content ?? "");
  return (
    <li className="run-step run-step-model">
      <div className="run-step-heading"><strong>Model</strong></div>
      {content && <p>{content}</p>}
    </li>
  );
}
