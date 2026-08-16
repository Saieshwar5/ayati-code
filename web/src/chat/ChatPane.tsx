import type { FormEvent, KeyboardEvent } from "react";
import { useEffect, useRef, useState } from "react";
import type { Message, Workspace, WorkspaceSession } from "../api/contracts";
import { statusLabel } from "../app/format";

interface ChatPaneProps {
  workspace: Workspace;
  session: WorkspaceSession;
  workspaceSessions: WorkspaceSession[];
  messages: Message[];
  error: string;
  sending: boolean;
  onSend: (text: string) => Promise<boolean>;
}

export function ChatPane(props: ChatPaneProps) {
  const [text, setText] = useState("");
  const textarea = useRef<HTMLTextAreaElement>(null);
  const messagesElement = useRef<HTMLDivElement>(null);
  const working = props.workspaceSessions.find((session) => session.status === "working");
  const enabled = props.workspace.status === "ready" && !working && !props.sending;
  const visibleMessages = props.messages.filter(isConversationMessage);
  const effectiveStatus = props.workspace.status === "ready" ? props.session.status : props.workspace.status;
  const detailError = props.workspace.error || props.session.error;

  useEffect(() => {
    setText("");
  }, [props.session.id]);

  useEffect(() => {
    const element = messagesElement.current;
    if (element) element.scrollTop = element.scrollHeight;
  }, [props.messages]);

  useEffect(() => resizeTextarea(textarea.current), [text]);

  async function submit(event?: FormEvent<HTMLFormElement>) {
    event?.preventDefault();
    if (!enabled || !text.trim()) return;
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
          <p className="eyebrow">{props.workspace.repository}</p>
          <h1>{props.session.title}</h1>
		  <p className="context-branch">
			{props.workspace.branch} · {props.workspace.authority === "explore" ? "Explore" : "Develop"}
		  </p>
        </div>
        <span
          className={`status ${effectiveStatus}`}
          title={statusLabel(effectiveStatus)}
          aria-label={`Status: ${statusLabel(effectiveStatus)}`}
        >
          {statusLabel(effectiveStatus)}
        </span>
      </div>
      {detailError && (
        <div className="workspace-alert error" role="alert">
          {detailError}
        </div>
      )}
      <div className="messages" ref={messagesElement}>
        {!visibleMessages.length && (
          <div className="conversation-empty muted">
            The environment is ready. Discuss the task, then send an explicit implementation request.
          </div>
        )}
        {visibleMessages.map((message, index) => (
          <div className={`message ${message.role}`} key={`${message.role}-${index}`}>
            {message.content}
          </div>
        ))}
      </div>
      <form className="composer" onSubmit={(event) => void submit(event)}>
        <textarea
          ref={textarea}
          rows={1}
          value={text}
          disabled={!enabled}
          placeholder={composerPlaceholder(props.workspace, props.session, working)}
          required
          onChange={(event) => setText(event.target.value)}
          onKeyDown={keyDown}
        />
        {props.error && (
          <span className="error" role="alert">
            {props.error}
          </span>
        )}
        <button
          className="composer-send"
          type="submit"
          disabled={!enabled || !text.trim()}
          aria-label="Send message"
          title="Send message"
        >
          <span aria-hidden="true">↑</span>
        </button>
      </form>
    </section>
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
  if (!working) return "Ask Ayati about this task…";
  return working.id === session.id
    ? "Ayati is working in this session…"
    : "Another session is working in this workspace…";
}

function resizeTextarea(element: HTMLTextAreaElement | null) {
  if (!element) return;
  element.style.height = "auto";
  const height = Math.min(element.scrollHeight, 144);
  element.style.height = `${height}px`;
  element.style.overflowY = element.scrollHeight > 144 ? "auto" : "hidden";
}
