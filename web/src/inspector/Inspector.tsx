import type { FormEvent } from "react";
import { useEffect, useState } from "react";
import type {
  Message,
  PublishInput,
  ToolCall,
  Workspace,
  WorkspaceSession,
} from "../api/contracts";

type InspectorPanel = "activity" | "changes" | "publish";

interface InspectorProps {
  collapsed: boolean;
  workspace?: Workspace;
  session?: WorkspaceSession;
  messages: Message[];
  changes: string;
  publishing: boolean;
  onCollapsedChange: (collapsed: boolean) => void;
  onRefreshChanges: () => Promise<void>;
  onPublish: (input: PublishInput) => Promise<boolean>;
}

export function Inspector(props: InspectorProps) {
  const [panel, setPanel] = useState<InspectorPanel>("activity");

  function selectPanel(next: InspectorPanel) {
    setPanel(next);
    if (next === "changes") void props.onRefreshChanges();
  }

  return (
    <aside className="inspector" aria-label="Workspace activity">
      <div className="inspector-heading">
        <div className="inspector-title">
          <p className="eyebrow">Internal work</p>
          <h2>{panel[0].toUpperCase() + panel.slice(1)}</h2>
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
          <p>Select a workspace to inspect agent activity and changes.</p>
        </div>
      ) : (
        <div className="inspector-content">
          <div className="inspector-tabs" role="tablist" aria-label="Workspace details">
            <InspectorTab name="activity" selected={panel} onSelect={selectPanel} subtitle="Session" />
            <InspectorTab name="changes" selected={panel} onSelect={selectPanel} subtitle="Workspace" />
            <InspectorTab name="publish" selected={panel} onSelect={selectPanel} subtitle="Workspace" />
          </div>
          {panel === "activity" && (
            <ActivityPanel workspace={props.workspace} session={props.session} messages={props.messages} />
          )}
          {panel === "changes" && (
            <section className="inspector-panel active" role="tabpanel">
              <div className="section-heading">
                <div>
                  <p className="eyebrow">Review</p>
                  <h3>Workspace changes</h3>
                </div>
                <button
                  className="quiet compact"
                  type="button"
                  onClick={() => void props.onRefreshChanges()}
                >
                  Refresh
                </button>
              </div>
              <p className="scope-note">Changes are shared by every session in this workspace.</p>
              <pre className="changes-output">{props.changes}</pre>
            </section>
          )}
          {panel === "publish" && (
            <PublishPanel
              workspace={props.workspace}
              publishing={props.publishing}
              onPublish={props.onPublish}
            />
          )}
        </div>
      )}
    </aside>
  );
}

interface InspectorTabProps {
  name: InspectorPanel;
  selected: InspectorPanel;
  subtitle: string;
  onSelect: (name: InspectorPanel) => void;
}

function InspectorTab({ name, selected, subtitle, onSelect }: InspectorTabProps) {
  const active = selected === name;
  return (
    <button
      className={`inspector-tab${active ? " active" : ""}`}
      type="button"
      role="tab"
      aria-selected={active}
      onClick={() => onSelect(name)}
    >
      {name[0].toUpperCase() + name.slice(1)}
      <span>{subtitle}</span>
    </button>
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
            Shell commands, results, and verification from this session will appear here.
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

interface PublishPanelProps {
  workspace: Workspace;
  publishing: boolean;
  onPublish: (input: PublishInput) => Promise<boolean>;
}

function PublishPanel({ workspace, publishing, onPublish }: PublishPanelProps) {
  const [commitMessage, setCommitMessage] = useState("feat: update project");
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    setTitle(workspace.branch.replaceAll("-", " ").replace(/^ayati\//, ""));
    setError("");
  }, [workspace.id, workspace.branch]);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    try {
      await onPublish({ commit_message: commitMessage, title, body });
    } catch (reason) {
      setError((reason as Error).message);
    }
  }

  return (
    <section className="inspector-panel active" role="tabpanel">
      <div className="section-heading">
        <div>
          <p className="eyebrow">GitHub</p>
          <h3>Publish changes</h3>
        </div>
      </div>
      <p className="scope-note">
        Publishing includes all current workspace changes, including work from other sessions.
      </p>
      {workspace.pull_request_url && (
        <a className="pull-link" href={workspace.pull_request_url} target="_blank" rel="noreferrer">
          Open pull request #{workspace.pull_request_number}
        </a>
      )}
      <form className="publish-form" onSubmit={(event) => void submit(event)}>
        <label>
          Commit message
          <input value={commitMessage} required onChange={(event) => setCommitMessage(event.target.value)} />
        </label>
        <label>
          Pull request title
          <input value={title} required onChange={(event) => setTitle(event.target.value)} />
        </label>
        <label>
          Pull request description
          <textarea
            value={body}
            rows={6}
            placeholder="What changed and how was it verified?"
            onChange={(event) => setBody(event.target.value)}
          />
        </label>
        {error && <div className="error">{error}</div>}
        <button className="primary" type="submit" disabled={publishing}>
          {workspace.pull_request_url ? "Push new changes" : "Create pull request"}
        </button>
      </form>
    </section>
  );
}

function activityState(workspace: Workspace, session: WorkspaceSession): string {
  if (workspace.status === "creating") return "Creating the workspace record and preparing initialization.";
  if (workspace.status === "initializing") return "Installing dependencies inside the persistent sandbox.";
  if (workspace.status === "initialization_failed") return workspace.error || "Workspace initialization failed.";
  if (workspace.status === "stopped") return "The persistent sandbox has been stopped.";
  if (session.status === "working") return "Ayati is working. New commands and results appear below.";
  if (session.status === "review") return "This session finished with workspace changes ready for review.";
  if (session.status === "failed") return session.error || "The last run in this session failed.";
  return "Fresh session context. Workspace files and changes are shared.";
}

function formatDuration(nanoseconds: number): string {
  const milliseconds = nanoseconds / 1e6;
  return milliseconds >= 1000 ? `${(milliseconds / 1000).toFixed(1)}s` : `${Math.round(milliseconds)}ms`;
}
