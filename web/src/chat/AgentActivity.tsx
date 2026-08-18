import { useEffect, useMemo, useState } from "react";
import type { Message, ToolCall } from "../api/contracts";

export interface ActivityStep {
  id: string;
  call: ToolCall;
  result?: Message;
}

export interface ActivityGroup {
  kind: "activity";
  id: string;
  closed: boolean;
  completed: boolean;
  steps: ActivityStep[];
}

export interface ConversationMessageItem {
  kind: "message";
  id: string;
  message: Message;
}

export type ConversationFeedItem = ActivityGroup | ConversationMessageItem;
export type ActivityRunState = "working" | "completed" | "failed" | "canceled" | "idle" | "review";

interface ShellResult {
  stdout?: string;
  stderr?: string;
  error?: string;
  exit_code: number;
  duration?: number;
  timed_out?: boolean;
  truncated?: boolean;
}

export function buildConversationFeed(messages: Message[]): ConversationFeedItem[] {
  const items: ConversationFeedItem[] = [];
  let activity: ActivityGroup | undefined;
  let requestID = "conversation";

  messages.forEach((message, index) => {
    const id = messageID(message, index);
    if (message.role === "user") {
      if (activity) activity.closed = true;
      activity = undefined;
      requestID = id;
      if (message.content) items.push({ kind: "message", id, message });
      return;
    }
    if (message.role === "assistant" && message.tool_calls?.length) {
      if (!activity) {
        activity = { kind: "activity", id: `activity-${requestID}`, closed: false, completed: false, steps: [] };
        items.push(activity);
      }
      message.tool_calls.forEach((call) => {
        if (!activity?.steps.some((step) => step.id === call.id)) {
          activity?.steps.push({ id: call.id, call });
        }
      });
      return;
    }
    if (message.role === "tool") {
      const step = activity?.steps.find((candidate) => candidate.id === message.tool_call_id);
      if (step) step.result = message;
      return;
    }
    if (message.role === "assistant") {
      if (activity) {
        activity.closed = true;
        activity.completed = true;
      }
      activity = undefined;
    }
    if (message.content) items.push({ kind: "message", id, message });
  });
  return items;
}

export function activeRequestID(messages: Message[]): string {
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    if (messages[index].role === "user") return `activity-${messageID(messages[index], index)}`;
  }
  return "activity-current";
}

export function AgentActivity(props: { group: ActivityGroup; state: ActivityRunState }) {
  const { group, state } = props;
  const latestStepID = group.steps.at(-1)?.id || "";
  const [timelineOpen, setTimelineOpen] = useState(state !== "completed" && state !== "idle");
  const [openStepID, setOpenStepID] = useState("");
  const parsed = useMemo(() => new Map(group.steps.map((step) => [step.id, parseResult(step.result)])), [group.steps]);
  const duration = [...parsed.values()].reduce((total, result) => total + (result?.duration || 0), 0);
  const summary = activitySummary(group.steps.length, state, duration);

  useEffect(() => {
    if (state === "working" || state === "failed" || state === "canceled") {
      setOpenStepID(latestStepID);
    } else {
      setOpenStepID("");
    }
  }, [latestStepID, state]);

  useEffect(() => {
    if (state === "completed" || state === "idle") setTimelineOpen(false);
    if (state === "failed" || state === "canceled") setTimelineOpen(true);
  }, [state]);

  return (
    <section className={`agent-activity ${state}`} aria-label="Agent activity">
      <button
        className="agent-activity-heading"
        type="button"
        aria-expanded={timelineOpen}
        aria-label={`Agent activity: ${summary}`}
        onClick={() => setTimelineOpen((open) => !open)}
      >
        <span className="agent-activity-pulse" aria-hidden="true" />
        <strong>Agent activity</strong>
        <small>{summary}</small>
        <span className="agent-activity-chevron" aria-hidden="true">⌄</span>
      </button>
      {timelineOpen && (
        !group.steps.length ? (
          <div className="agent-activity-starting"><span aria-hidden="true" />Preparing the first step…</div>
        ) : (
          <div className="agent-step-list">
            {group.steps.map((step, index) => {
              const command = shellCommand(step.call);
              const result = parsed.get(step.id);
              const status = stepStatus(result, state);
              const open = openStepID === step.id;
              return (
                <article className={`agent-step ${status}`} key={step.id}>
                  <button
                    className="agent-step-summary"
                    type="button"
                    aria-expanded={open}
                    onClick={() => setOpenStepID(open ? "" : step.id)}
                  >
                    <span className="agent-step-marker" aria-hidden="true">{stepMarker(status)}</span>
                    <span><strong>Step {index + 1} · {stepLabel(command)}</strong><code>{shortCommand(command)}</code></span>
                    <small>{stepStatusLabel(status, result)}</small>
                    <span className="agent-step-chevron" aria-hidden="true">⌄</span>
                  </button>
                  {open && <StepDetail command={command} result={result} status={status} />}
                </article>
              );
            })}
          </div>
        )
      )}
    </section>
  );
}

function StepDetail(props: { command: string; result?: ShellResult; status: string }) {
  const output = props.result
    ? [props.result.stdout, props.result.stderr, props.result.error].filter(Boolean).join("\n") || "Command completed without output."
    : props.status === "running" ? "Command is running…" : "No command result was recorded.";
  return (
    <div className="agent-step-detail">
      <div><span>Command</span><pre>$ {props.command}</pre></div>
      <div><span>Result</span><pre>{output}</pre></div>
      {props.result?.truncated && <p>Output was truncated to the workspace command limit.</p>}
    </div>
  );
}

function shellCommand(call: ToolCall): string {
  try {
    return (JSON.parse(call.function.arguments) as { command?: string }).command?.trim() || call.function.arguments;
  } catch {
    return call.function.arguments;
  }
}

function parseResult(message?: Message): ShellResult | undefined {
  if (!message) return undefined;
  try {
    return JSON.parse(message.content || "") as ShellResult;
  } catch {
    return { stdout: message.content || "", exit_code: 1, error: "Invalid shell result" };
  }
}

function stepStatus(result: ShellResult | undefined, state: ActivityRunState): string {
  if (result) return result.exit_code === 0 && !result.error && !result.timed_out ? "completed" : "failed";
  if (state === "working") return "running";
  if (state === "canceled") return "canceled";
  return "failed";
}

function stepStatusLabel(status: string, result?: ShellResult): string {
  if (status === "running") return "Running";
  if (status === "canceled") return "Stopped";
  if (status === "failed") return result?.timed_out ? "Timed out" : `Failed${result ? ` · exit ${result.exit_code}` : ""}`;
  return `Done${result?.duration ? ` · ${formatDuration(result.duration)}` : ""}`;
}

function stepMarker(status: string): string {
  if (status === "completed") return "✓";
  if (status === "failed") return "!";
  if (status === "canceled") return "■";
  return "";
}

function activitySummary(count: number, state: ActivityRunState, duration: number): string {
  const elapsed = duration ? ` · ${formatDuration(duration)}` : "";
  if (state === "working") return count ? `${count} ${count === 1 ? "step" : "steps"} · Working` : "Starting";
  if (state === "failed") return `Failed · ${count} ${count === 1 ? "step" : "steps"}${elapsed}`;
  if (state === "canceled") return `Stopped · ${count} ${count === 1 ? "step" : "steps"}${elapsed}`;
  if (state === "completed") return `Completed · ${count} ${count === 1 ? "step" : "steps"}${elapsed}`;
  return `${count} ${count === 1 ? "step" : "steps"}${elapsed}`;
}

function stepLabel(command: string): string {
  if (/^(?:rg|grep|find)\b/.test(command)) return "Search project";
  if (/^(?:cat|sed|head|tail)\b/.test(command)) return "Read project files";
  if (/^git\s+diff\b/.test(command)) return "Review changes";
  if (/^git\s+status\b/.test(command)) return "Check workspace status";
  if (/\b(?:go test|npm test|npm run test|pnpm test|pytest|cargo test)\b/.test(command)) return "Run tests";
  if (/\b(?:go build|npm run build|pnpm build|cargo build)\b/.test(command)) return "Build project";
  if (/\b(?:npm ci|npm install|pnpm install|go mod download|uv sync)\b/.test(command)) return "Install dependencies";
  return "Run shell command";
}

function shortCommand(command: string): string {
  const singleLine = command.replace(/\s+/g, " ").trim();
  return singleLine.length > 76 ? `${singleLine.slice(0, 73)}…` : singleLine;
}

function formatDuration(nanoseconds: number): string {
  const milliseconds = nanoseconds / 1e6;
  return milliseconds >= 1000 ? `${(milliseconds / 1000).toFixed(1)}s` : `${Math.round(milliseconds)}ms`;
}

function messageID(message: Message, index: number): string {
  return message.id === undefined ? `position-${index}` : String(message.id);
}
