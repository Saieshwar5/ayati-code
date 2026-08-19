import { useEffect, useRef, useState } from "react";
import type { ConversationContextEntry } from "./useConversationContexts";

interface ComposerContextControlProps {
  currentMessageCount: number;
  history: ConversationContextEntry[];
  disabled: boolean;
  taskMode: boolean;
  onOpen: () => void;
  onStartFresh: () => void;
  onViewHistory: (contextID: string) => void;
}

type CompactState = "idle" | "working" | "done";

export function ComposerContextControl(props: ComposerContextControlProps) {
  const [open, setOpen] = useState(false);
  const [compactState, setCompactState] = useState<CompactState>("idle");
  const compactTimer = useRef<number | undefined>(undefined);

  useEffect(() => {
    if (props.taskMode) setOpen(false);
  }, [props.taskMode]);

  useEffect(() => () => window.clearTimeout(compactTimer.current), []);

  function toggle() {
    const next = !open;
    setOpen(next);
    if (next) props.onOpen();
  }

  function startFresh() {
    props.onStartFresh();
    setOpen(false);
    setCompactState("idle");
  }

  function viewHistory(contextID: string) {
    props.onViewHistory(contextID);
    setOpen(false);
  }

  function compact() {
    setCompactState("working");
    compactTimer.current = window.setTimeout(() => {
      setCompactState("done");
      compactTimer.current = window.setTimeout(() => {
        setOpen(false);
        setCompactState("idle");
      }, 550);
    }, 750);
  }

  const actionDisabled = props.disabled || props.currentMessageCount === 0 || compactState !== "idle";
  return (
    <div className="composer-context-control">
      <button
        className={`composer-tool-button${open ? " active" : ""}`}
        type="button"
        aria-expanded={open}
        aria-label={open ? "Close context controls" : "Open context controls"}
        onClick={toggle}
      >
        Context <span aria-hidden="true">⌄</span>
      </button>
      {open && (
        <section className="composer-context-tray" aria-label="Context controls">
          <article className={`context-current-row ${compactState}`} aria-label="Current conversation">
            <i aria-hidden="true" />
            <div>
              <strong>Current conversation</strong>
              <span>{currentStatus(props.currentMessageCount, compactState)}</span>
            </div>
            <button className="context-compact-action" type="button" disabled={actionDisabled} onClick={compact}>
              {compactState === "working" ? "Compacting…" : compactState === "done" ? "✓ Compacted" : "Compact"}
            </button>
            {compactState === "working" && <span className="context-compact-progress" aria-hidden="true" />}
          </article>

          <button className="context-fresh-row" type="button" disabled={actionDisabled} onClick={startFresh}>
            <span aria-hidden="true">+</span>
            <span><strong>Start fresh conversation</strong><small>Continue without previous agent context</small></span>
          </button>

          <div className="context-history">
            <div className="context-history-heading"><strong>Past conversations</strong><span>{props.history.length}</span></div>
            <div className="context-history-list">
              {props.history.length ? props.history.map((entry) => (
                <button type="button" key={entry.id} onClick={() => viewHistory(entry.id)}>
                  <span><strong>{entry.title}</strong><small>{messageCountLabel(entry.messages.length)}</small></span>
                  <time dateTime={entry.createdAt.toISOString()}>{relativeTime(entry.createdAt)}</time>
                </button>
              )) : <p className="context-history-empty">No past conversations.</p>}
            </div>
          </div>
          <small className="context-preview-note">UI preview · Runtime connection pending</small>
        </section>
      )}
    </div>
  );
}

function currentStatus(messageCount: number, state: CompactState): string {
  if (state === "working") return "Summarizing earlier messages";
  if (state === "done") return "Context compacted";
  if (!messageCount) return "No messages yet";
  return `${messageCountLabel(messageCount)} · Automatic compaction on`;
}

function messageCountLabel(count: number): string {
  return `${count} ${count === 1 ? "message" : "messages"}`;
}

function relativeTime(value: Date): string {
  return Date.now() - value.getTime() < 60_000
    ? "Just now"
    : value.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
}
