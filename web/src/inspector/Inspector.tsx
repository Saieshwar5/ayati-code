import type {
  Message,
  ToolCall,
  Workspace,
  WorkspaceSession,
} from "../api/contracts";

interface InspectorProps {
  collapsed: boolean;
  workspace?: Workspace;
  session?: WorkspaceSession;
  messages: Message[];
  onCollapsedChange: (collapsed: boolean) => void;
}

export function Inspector(props: InspectorProps) {
  return (
    <aside className="inspector" aria-label="Agent activity">
      <div className="inspector-heading">
        <div className="inspector-title">
          <p className="eyebrow">Internal work</p>
          <h2>Activity</h2>
        </div>
        <button
          className="icon-button"
          type="button"
          aria-expanded={!props.collapsed}
          aria-label={props.collapsed ? "Open internal work" : "Collapse internal work"}
          onClick={() => props.onCollapsedChange(!props.collapsed)}
        >
          {props.collapsed ? "‹" : "›"}
        </button>
      </div>

      {!props.workspace || !props.session ? (
        <div className="inspector-empty">
          <p>Open a conversation to inspect shell commands and verification.</p>
        </div>
      ) : (
        <div className="inspector-content">
          <ActivityPanel workspace={props.workspace} session={props.session} messages={props.messages} />
        </div>
      )}
    </aside>
  );
}

interface ActivityPanelProps {
  workspace: Workspace;
  session: WorkspaceSession;
  messages: Message[];
}

function ActivityPanel({ workspace, session, messages }: ActivityPanelProps) {
  const entries = activityEntries(messages);
  const failed = session.status === "failed" || workspace.status === "initialization_failed";
  return (
    <section className="inspector-panel active" role="tabpanel">
      <div className={`activity-state${session.status === "working" ? " working" : ""}${failed ? " failed" : ""}`}>
        {activityState(workspace, session)}
      </div>
      <div className="activity-list">
        {entries.length ? (
          entries
        ) : (
          <p className="activity-empty">
            Shell commands, results, and verification from this conversation will appear here.
          </p>
        )}
      </div>
    </section>
  );
}

function activityEntries(messages: Message[]) {
  const entries: React.ReactNode[] = [];
  messages.forEach((message, messageIndex) => {
    if (message.role === "assistant") {
      message.tool_calls?.forEach((call, callIndex) =>
        entries.push(<ToolCallEntry key={`call-${messageIndex}-${callIndex}`} call={call} />),
      );
    }
    if (message.role === "tool") {
      entries.push(<ToolResultEntry key={`result-${messageIndex}`} content={message.content || ""} />);
    }
  });
  return entries;
}

function ToolCallEntry({ call }: { call: ToolCall }) {
  let command = call.function?.arguments || "";
  try {
    command = (JSON.parse(command) as { command?: string }).command || command;
  } catch {
    // Raw arguments are still useful when a tool call is malformed.
  }
  return (
    <article className="activity-entry command">
      <div className="activity-entry-heading">
        <span>Shell command</span>
        <span>shell</span>
      </div>
      <pre>$ {command}</pre>
    </article>
  );
}

interface ShellResult {
  stdout?: string;
  stderr?: string;
  error?: string;
  exit_code: number;
  duration?: number;
  timed_out?: boolean;
}

function ToolResultEntry({ content }: { content: string }) {
  let result: ShellResult;
  try {
    result = JSON.parse(content) as ShellResult;
  } catch {
    result = { stdout: content, exit_code: 1 };
  }
  const failed = result.exit_code !== 0 || result.error || result.timed_out;
  const output = [result.stdout, result.stderr, result.error].filter(Boolean).join("\n") || "No output.";
  const duration = result.duration ? ` · ${formatDuration(result.duration)}` : "";
  return (
    <details className={`activity-entry result${failed ? " failed" : ""}`} open={Boolean(failed || output.length < 500)}>
      <summary className="activity-entry-heading">
        <span>Command result</span>
        <span>
          exit {result.exit_code}
          {duration}
        </span>
      </summary>
      <pre>{output}</pre>
    </details>
  );
}

function activityState(workspace: Workspace, session: WorkspaceSession): string {
  if (workspace.status === "creating") return "Creating the workspace record and preparing initialization.";
  if (workspace.status === "initializing") return "Installing dependencies inside the persistent sandbox.";
  if (workspace.status === "initialization_failed") return workspace.error || "Workspace initialization failed.";
  if (workspace.status === "stopped") return "The persistent sandbox has been stopped.";
  if (session.status === "working") return "Ayati is working. New commands and results appear below.";
  if (session.status === "review") return "This conversation finished with workspace changes ready for review.";
  if (session.status === "failed") return session.error || "The last run in this conversation failed.";
  if (session.status === "canceled") return "The last run was stopped by the user.";
  return "Fresh conversation context. Workspace files and changes are shared.";
}

function formatDuration(nanoseconds: number): string {
  const milliseconds = nanoseconds / 1e6;
  return milliseconds >= 1000 ? `${(milliseconds / 1000).toFixed(1)}s` : `${Math.round(milliseconds)}ms`;
}
