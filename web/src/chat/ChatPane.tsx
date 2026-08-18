import type { FormEvent, KeyboardEvent } from "react";
import { useEffect, useRef, useState } from "react";
import type { AgentDefinition, Message, Workspace, WorkspaceSession } from "../api/contracts";
import { repositoryName, statusLabel } from "../app/format";
import type { WorkspacePanelSection } from "../workspaces/WorkspacePanel";
import { ComposerContextControl } from "./ComposerContextControl";
import { MarkdownMessage } from "./MarkdownMessage";
import { useConversationContexts } from "./useConversationContexts";

interface ChatPaneProps {
  workspace: Workspace;
  session: WorkspaceSession;
  workspaceSessions: WorkspaceSession[];
  messages: Message[];
  error: string;
  sending: boolean;
  stopping: boolean;
  agents: AgentDefinition[];
  onSend: (text: string) => Promise<boolean>;
  onStop: () => Promise<boolean>;
  onSelectAgent: (agentID: string) => Promise<void>;
  onCreateTask?: (request: string) => void;
  taskCount?: number;
  workspacePanelOpen?: boolean;
  workspacePanelSection?: WorkspacePanelSection;
  onOpenWorkspacePanel?: (section: WorkspacePanelSection) => void;
  onResumeWorkspace?: () => void;
}

export function ChatPane(props: ChatPaneProps) {
  const [text, setText] = useState("");
  const [taskMode, setTaskMode] = useState(false);
  const textarea = useRef<HTMLTextAreaElement>(null);
  const messagesElement = useRef<HTMLDivElement>(null);
  const working = props.workspaceSessions.find((session) => session.status === "working");
  const currentSessionWorking = working?.id === props.session.id || props.sending;
  const enabled = props.workspace.status === "ready" && !working && !props.sending;
  const conversationMessages = props.messages.filter(isConversationMessage);
  const contexts = useConversationContexts(props.session.id, conversationMessages);
  const visibleMessages = contexts.displayedMessages;
  const effectiveStatus = props.workspace.status === "ready" ? props.session.status : props.workspace.status;
  const detailError = props.workspace.error || (props.session.status === "failed" ? props.session.error : "");

  useEffect(() => {
    setText("");
  }, [props.session.id]);

  useEffect(() => {
    const element = messagesElement.current;
    if (element) element.scrollTop = element.scrollHeight;
  }, [props.messages, contexts.viewedEntry?.id]);

  useEffect(() => resizeTextarea(textarea.current), [text]);

  async function submit(event?: FormEvent<HTMLFormElement>) {
    event?.preventDefault();
    if (!enabled || !text.trim()) return;
    if (taskMode && props.onCreateTask) {
      props.onCreateTask(text.trim());
      setText("");
      setTaskMode(false);
      return;
    }
    if (await props.onSend(text)) setText("");
  }

  function keyDown(event: KeyboardEvent<HTMLTextAreaElement>) {
    if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
      event.preventDefault();
      void submit();
    }
  }

  return (
    <section className="workspace-detail">
      <div className="conversation-heading">
        <div className="workspace-context">
          <h1>{repositoryName(props.workspace.repository)}</h1>
          <p className="context-branch">
            Conversation · {props.workspace.branch}
          </p>
        </div>
        <div className="conversation-heading-actions">
          {props.onOpenWorkspacePanel && (
            <>
              <PanelButton section="tasks" label={`Tasks${props.taskCount ? ` ${props.taskCount}` : ""}`} open={props.workspacePanelOpen} selected={props.workspacePanelSection} onOpen={props.onOpenWorkspacePanel} />
              <PanelButton section="changes" label="Changes" open={props.workspacePanelOpen} selected={props.workspacePanelSection} onOpen={props.onOpenWorkspacePanel} />
              <PanelButton section="workspace" label="Workspace" open={props.workspacePanelOpen} selected={props.workspacePanelSection} onOpen={props.onOpenWorkspacePanel} />
            </>
          )}
          <span className={`status ${effectiveStatus}`} title={statusLabel(effectiveStatus)} aria-label={`Status: ${statusLabel(effectiveStatus)}`}>{statusLabel(effectiveStatus)}</span>
        </div>
      </div>
      {detailError && (
        <div className="workspace-alert error" role="alert">
          {detailError}
        </div>
      )}
      <div className="messages" ref={messagesElement}>
        {contexts.viewedEntry && (
          <div className="past-conversation-heading">
            <div><p className="eyebrow">Past conversation</p><strong>{contexts.viewedEntry.title}</strong></div>
            <button className="quiet compact" type="button" onClick={contexts.returnToCurrent}>Return to current</button>
          </div>
        )}
        {props.workspace.status !== "ready" && (
          <WorkspaceStateEvent workspace={props.workspace} onResume={props.onResumeWorkspace} onOpenDetails={() => props.onOpenWorkspacePanel?.("workspace")} />
        )}
        {!visibleMessages.length && props.workspace.status === "ready" && (
          <div className="conversation-empty muted">
            {contexts.history.length
              ? "Fresh conversation started. Previous conversations remain available from Context."
              : "Discuss the project, clarify intent, or create a durable task when the direction is ready."}
          </div>
        )}
        {visibleMessages.map((message, index) => (
          <div className={`message-entry ${message.role}`} key={`${message.role}-${index}`}>
            {message.role === "assistant" && message.agent && (
              <div className="message-agent"><span aria-hidden="true">{message.agent.emoji}</span><strong>{message.agent.name}</strong></div>
            )}
            <div className={`message ${message.role}`}>
              {message.role === "assistant" ? (
                <MarkdownMessage content={message.content || ""} />
              ) : (
                message.content
              )}
            </div>
          </div>
        ))}
      </div>
      {contexts.viewedEntry ? (
        <div className="past-conversation-bar">
          <span>Viewing history · messages are read-only</span>
          <button className="primary compact" type="button" onClick={contexts.returnToCurrent}>Return to current conversation</button>
        </div>
      ) : <form className={`composer${taskMode ? " task-mode" : ""}`} onSubmit={(event) => void submit(event)}>
        <textarea
          ref={textarea}
          rows={1}
          value={text}
          disabled={!enabled}
          placeholder={taskMode ? "Describe the task or tasks to create…" : composerPlaceholder(props.workspace, props.session, working)}
          required
          onChange={(event) => setText(event.target.value)}
          onKeyDown={keyDown}
        />
        {props.error && (
          <span className="error" role="alert">
            {props.error}
          </span>
        )}
        {currentSessionWorking ? (
          <button
            className="composer-send composer-stop"
            type="button"
            disabled={props.stopping}
            aria-label="Stop agent run"
            title={props.stopping ? "Stopping agent run" : "Stop agent run"}
            onClick={() => void props.onStop()}
          >
            <span className="stop-icon" aria-hidden="true" />
          </button>
        ) : (
          <button
            className="composer-send"
            type="submit"
            disabled={!enabled || !text.trim()}
            aria-label={taskMode ? "Create task" : "Send message"}
            title={taskMode ? "Create task" : "Send message"}
          >
            <span aria-hidden="true">↑</span>
          </button>
        )}
        <div className="composer-toolbar">
          <label className="agent-picker">
            <span className="sr-only">Agent</span>
            <select
              aria-label="Agent"
              value={props.session.selected_agent_id}
              disabled={taskMode || !enabled || props.agents.length === 0}
              onChange={(event) => void props.onSelectAgent(event.target.value)}
            >
              {props.agents.map((definition) => (
                <option key={definition.id} value={definition.id}>
                  {definition.emoji} {definition.name}{definition.default ? " · Default" : ""}
                </option>
              ))}
            </select>
          </label>
          <ComposerContextControl
            key={props.session.id}
            currentMessageCount={contexts.activeMessages.length}
            history={contexts.history}
            disabled={!enabled}
            taskMode={taskMode}
            onOpen={() => setTaskMode(false)}
            onStartFresh={contexts.startFresh}
            onViewHistory={contexts.viewHistory}
          />
          {props.onCreateTask && (
            <button className={`composer-task-toggle${taskMode ? " active" : ""}`} type="button" aria-pressed={taskMode} onClick={() => setTaskMode((current) => !current)}>
              {taskMode ? "Task mode" : "Create task"}
            </button>
          )}
        </div>
      </form>}
    </section>
  );
}

function WorkspaceStateEvent(props: { workspace: Workspace; onResume?: () => void; onOpenDetails: () => void }) {
  const status = props.workspace.status;
  const preparing = status === "creating" || status === "initializing";
  const title = preparing ? "Preparing workspace" : status === "stopped" ? "Workspace stopped" : "Workspace needs attention";
  const detail = preparing
    ? props.workspace.preparation_detail || "Preparing the repository and development environment."
    : status === "stopped"
      ? "Conversation history and project files are preserved. Resume to continue working."
      : props.workspace.error || "Open workspace details to continue preparation.";
  return (
    <section className="conversation-workspace-event" aria-label={title}>
      <span className={preparing ? "working" : ""} aria-hidden="true" />
      <div><p className="eyebrow">Workspace</p><h2>{title}</h2><p>{detail}</p></div>
      {status === "stopped" && props.onResume ? <button className="primary compact" type="button" onClick={props.onResume}>Resume</button> : <button className="quiet compact" type="button" onClick={props.onOpenDetails}>View details</button>}
    </section>
  );
}

function PanelButton(props: {
  section: WorkspacePanelSection;
  label: string;
  open?: boolean;
  selected?: WorkspacePanelSection;
  onOpen: (section: WorkspacePanelSection) => void;
}) {
  const active = Boolean(props.open && props.selected === props.section);
  return (
    <button
      className={`conversation-panel-button${active ? " active" : ""}`}
      type="button"
      aria-pressed={active}
      onClick={() => props.onOpen(props.section)}
    >
      {props.label}
    </button>
  );
}

function isConversationMessage(message: Message): boolean {
  if (message.role === "tool") return false;
  if (message.role === "assistant" && message.tool_calls?.length) return false;
  return Boolean(message.content);
}

function composerPlaceholder(
  workspace: Workspace,
  session: WorkspaceSession,
  working?: WorkspaceSession,
): string {
  if (workspace.status !== "ready") return `Workspace is ${statusLabel(workspace.status)}…`;
  if (!working) return "Ask perpetual about this task…";
  return working.id === session.id
    ? "perpetual is working in this conversation…"
    : "Another conversation is working in this workspace…";
}

function resizeTextarea(element: HTMLTextAreaElement | null) {
  if (!element) return;
  element.style.height = "auto";
  const height = Math.min(element.scrollHeight, 144);
  element.style.height = `${height}px`;
  element.style.overflowY = element.scrollHeight > 144 ? "auto" : "hidden";
}
