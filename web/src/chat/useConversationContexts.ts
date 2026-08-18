import { useEffect, useState } from "react";
import type { Message } from "../api/contracts";

export interface ConversationContextEntry {
  id: string;
  title: string;
  messages: Message[];
  createdAt: Date;
}

export function useConversationContexts(sessionID: string, messages: Message[]) {
  const [activeStartIndex, setActiveStartIndex] = useState(0);
  const [history, setHistory] = useState<ConversationContextEntry[]>([]);
  const [viewingHistoryID, setViewingHistoryID] = useState("");
  const activeMessages = messages.slice(activeStartIndex);
  const viewedEntry = history.find((entry) => entry.id === viewingHistoryID);

  useEffect(() => {
    setActiveStartIndex(0);
    setHistory([]);
    setViewingHistoryID("");
  }, [sessionID]);

  function startFresh() {
    if (!activeMessages.length) return;
    setHistory((current) => [{
      id: `context-${Date.now()}-${current.length}`,
      title: contextTitle(activeMessages),
      messages: activeMessages,
      createdAt: new Date(),
    }, ...current]);
    setActiveStartIndex(messages.length);
    setViewingHistoryID("");
  }

  return {
    activeMessages,
    displayedMessages: viewedEntry?.messages || activeMessages,
    history,
    viewedEntry,
    startFresh,
    viewHistory: setViewingHistoryID,
    returnToCurrent: () => setViewingHistoryID(""),
  };
}

function contextTitle(messages: Message[]): string {
  const latestUserMessage = [...messages].reverse()
    .find((message) => message.role === "user" && message.content)?.content || "Conversation context";
  const title = latestUserMessage.trim().split("\n")[0].replace(/^#+\s*/, "");
  return title.length > 64 ? `${title.slice(0, 61)}…` : title;
}
