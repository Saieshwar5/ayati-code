import { api, state, ui } from "./shared.js";
import {
  initializeInspector, renderActivityEmpty, renderActivityMessage,
} from "./inspector.js";
import { loadEnvironment, syncEnvironmentAvailability } from "./environment.js";

export async function openWorkspace(workspaceID, sessionID) {
  const workspace = state.workspaces.find((item) => item.id === workspaceID);
  let session = (state.sessions[workspaceID] || []).find((item) => item.id === sessionID);
  if (!workspace) return;
  if (!session) session = await api(`/api/workspaces/${workspaceID}/sessions/${sessionID}`);
  const changedWorkspace = state.activeWorkspace?.id !== workspaceID;
  state.activeSession = session;
  syncActiveWorkspace(workspace);
  ui.home.classList.add("hidden");
  ui.detail.classList.remove("hidden");
  ui.inspectorEmpty.classList.add("hidden");
  ui.inspectorContent.classList.remove("hidden");
  if (changedWorkspace) {
    ui.pullTitle.value = workspace.branch.replaceAll("-", " ").replace(/^ayati\//, "");
  }
  renderPullRequest();
  await Promise.all([loadMessages(), loadChanges(), loadEnvironment()]);
}
export function syncActiveWorkspace(workspace) {
  state.activeWorkspace = workspace;
  ui.detailRepository.textContent = workspace.repository;
  ui.detailBranch.textContent = workspace.branch;
  ui.detailSessionTitle.textContent = state.activeSession?.title || "Session";
  const status = workspace.status === "ready" ? state.activeSession?.status || "idle" : workspace.status;
  ui.detailStatus.textContent = status.replaceAll("_", " ");
  ui.detailStatus.className = `status ${status}`;
  ui.detailStatus.title = status.replaceAll("_", " ");
  ui.detailStatus.setAttribute("aria-label", `Status: ${status.replaceAll("_", " ")}`);
  const message = workspace.error || state.activeSession?.error || "";
  ui.detailError.textContent = message;
  ui.detailError.classList.toggle("hidden", !message);
  renderActivityState(workspace, state.activeSession);
  syncEnvironmentAvailability();
  renderPullRequest();
  syncComposer(workspace);
  markActiveNavigation(workspace);
}
function markActiveNavigation(workspace) {
  document.querySelectorAll(".workspace-item").forEach((item) => item.classList.remove("active"));
  const item = document.querySelector(`[data-workspace-id="${CSS.escape(workspace.id)}"]`);
  if (!item) return;
  item.classList.add("active");
  item.querySelectorAll(".session-item").forEach((row) => {
    row.classList.toggle("active", row.dataset.sessionId === state.activeSession?.id);
  });
}
function syncComposer(workspace) {
  const working = (state.sessions[workspace.id] || []).find((session) => session.status === "working");
  const enabled = workspace.status === "ready" && !working;
  ui.message.disabled = !enabled;
  ui.sendMessage.disabled = !enabled;
  if (enabled) {
    ui.message.placeholder = "Ask Ayati about this task…";
  } else if (working) {
    ui.message.placeholder = working.id === state.activeSession?.id
      ? "Ayati is working in this session…" : "Another session is working in this workspace…";
  } else {
    ui.message.placeholder = `Workspace is ${workspace.status.replaceAll("_", " ")}…`;
  }
}
function renderActivityState(workspace, session) {
  const workspaceDescriptions = {
    creating: "Creating the workspace record and preparing initialization.",
    initializing: "Installing dependencies inside the persistent sandbox.",
    initialization_failed: workspace.error || "Workspace initialization failed.",
    ready: "Environment ready.",
    stopped: "The persistent sandbox has been stopped.",
  };
  const sessionDescriptions = {
    idle: "Fresh session context. Workspace files and changes are shared.",
    working: "Ayati is working. New commands and results appear below.",
    review: "This session finished with workspace changes ready for review.",
    failed: session?.error || "The last run in this session failed.",
  };
  ui.activityState.textContent = workspace.status === "ready"
    ? sessionDescriptions[session?.status] || "Session ready."
    : workspaceDescriptions[workspace.status] || workspace.status;
  ui.activityState.className = "activity-state";
  if (session?.status === "working") ui.activityState.classList.add("working");
  if (session?.status === "failed" || workspace.status === "initialization_failed") ui.activityState.classList.add("failed");
}
async function loadChanges() {
  if (!state.activeWorkspace) return;
  const workspaceID = state.activeWorkspace.id;
  const workspaceStatus = state.activeWorkspace.status;
  if (workspaceStatus !== "ready") {
    ui.changes.textContent = `Changes are available after the workspace is ready.\n\nCurrent status: ${workspaceStatus.replaceAll("_", " ")}`;
    return;
  }
  ui.changes.textContent = "Loading changes…";
  try {
    const changes = await api(`/api/workspaces/${workspaceID}/changes`);
    if (state.activeWorkspace?.id !== workspaceID) return;
    ui.changes.textContent = [changes.status, changes.diff].filter(Boolean).join("\n") || "Working tree is clean.";
  } catch (error) {
    if (state.activeWorkspace?.id !== workspaceID) return;
    ui.changes.textContent = error.message;
  }
}
function renderPullRequest() {
  const hasPull = Boolean(state.activeWorkspace?.pull_request_url);
  ui.pullLink.classList.toggle("hidden", !hasPull);
  const submit = ui.publishForm.querySelector("button[type=submit]");
  if (hasPull) {
    ui.pullLink.href = state.activeWorkspace.pull_request_url;
    ui.pullLink.textContent = `Open pull request #${state.activeWorkspace.pull_request_number}`;
    submit.textContent = "Push new changes";
  } else {
    submit.textContent = "Create pull request";
  }
}
async function publish(event) {
  event.preventDefault();
  ui.publishError.classList.add("hidden");
  const submit = ui.publishForm.querySelector("button[type=submit]");
  submit.disabled = true;
  try {
    const workspace = await api(`/api/workspaces/${state.activeWorkspace.id}/publish`, {
      method: "POST",
      body: JSON.stringify({
        commit_message: ui.commitMessage.value,
        title: ui.pullTitle.value,
        body: ui.pullBody.value,
      }),
    });
    syncActiveWorkspace(workspace);
    renderPullRequest();
    await loadChanges();
  } catch (error) {
    ui.publishError.textContent = error.message;
    ui.publishError.classList.remove("hidden");
  } finally {
    submit.disabled = false;
  }
}
async function loadMessages() {
  if (!state.activeWorkspace || !state.activeSession) return;
  const workspaceID = state.activeWorkspace.id;
  const sessionID = state.activeSession.id;
  try {
    const messages = await api(`/api/workspaces/${workspaceID}/sessions/${sessionID}/messages`);
    if (state.activeWorkspace?.id !== workspaceID || state.activeSession?.id !== sessionID) return;
    ui.messages.replaceChildren();
    ui.activity.replaceChildren();
    for (const message of messages) renderMessage(message);
    if (!ui.messages.children.length) renderConversationEmpty();
    if (!ui.activity.children.length) renderActivityEmpty();
    ui.messages.scrollTop = ui.messages.scrollHeight;
    ui.activityPanel.scrollTop = ui.activityPanel.scrollHeight;
  } catch (error) {
    if (state.activeWorkspace?.id !== workspaceID || state.activeSession?.id !== sessionID) return;
    showMessageError(error.message);
  }
}

function renderConversationEmpty() {
  const empty = document.createElement("div");
  empty.className = "conversation-empty muted";
  empty.textContent = "The environment is ready. Discuss the task, then send an explicit implementation request.";
  ui.messages.append(empty);
}

function renderMessage(message) {
  if (renderActivityMessage(message)) return;
  const element = document.createElement("div");
  element.className = `message ${message.role}`;
  element.textContent = message.content;
  ui.messages.append(element);
}

async function refreshSession(workspaceID, sessionID) {
  const [workspaces, session] = await Promise.all([
    api("/api/workspaces"), api(`/api/workspaces/${workspaceID}/sessions/${sessionID}`),
  ]);
  state.workspaces = workspaces;
  const workspace = workspaces.find((item) => item.id === workspaceID);
  const sessions = state.sessions[workspaceID] || [];
  const index = sessions.findIndex((item) => item.id === session.id);
  if (index >= 0) sessions[index] = session;
  window.dispatchEvent(new CustomEvent("ayati:session-updated", { detail: session }));
  if (!workspace || state.activeWorkspace?.id !== workspaceID || state.activeSession?.id !== sessionID) return;
  state.activeSession = session;
  syncActiveWorkspace(workspace);
}

function startMessagePolling(workspaceID, sessionID) {
  stopMessagePolling();
  state.messagePollTimer = setInterval(async () => {
    if (state.messagePollBusy) return;
    state.messagePollBusy = true;
    try {
      const active = state.activeWorkspace?.id === workspaceID && state.activeSession?.id === sessionID;
      await Promise.all([active ? loadMessages() : Promise.resolve(), refreshSession(workspaceID, sessionID)]);
    } finally {
      state.messagePollBusy = false;
    }
  }, 800);
}

function stopMessagePolling() {
  clearInterval(state.messagePollTimer);
  state.messagePollTimer = null;
}

async function sendMessage() {
  const text = ui.message.value.trim();
  if (!text) return;
  showMessageError();
  ui.sendMessage.disabled = true;
  const workspaceID = state.activeWorkspace.id;
  const sessionID = state.activeSession.id;
  startMessagePolling(workspaceID, sessionID);
  try {
    await api(`/api/workspaces/${workspaceID}/sessions/${sessionID}/messages`, {
      method: "POST", body: JSON.stringify({ text }),
    });
    ui.message.value = "";
    resizeComposer();
  } catch (error) {
    showMessageError(error.message);
  } finally {
    stopMessagePolling();
    const active = state.activeWorkspace?.id === workspaceID && state.activeSession?.id === sessionID;
    try {
      await Promise.all([
        active ? loadMessages() : Promise.resolve(),
        refreshSession(workspaceID, sessionID),
        active ? loadChanges() : Promise.resolve(),
      ]);
    } catch (error) {
      if (state.activeWorkspace?.id === workspaceID) showMessageError(error.message);
    }
    if (state.activeWorkspace && state.activeSession) syncComposer(state.activeWorkspace);
  }
}

function showMessageError(message = "") {
  ui.messageError.textContent = message;
  ui.messageError.classList.toggle("hidden", !message);
}

function resizeComposer() {
  ui.message.style.height = "auto";
  const height = Math.min(ui.message.scrollHeight, 144);
  ui.message.style.height = `${height}px`;
  ui.message.style.overflowY = ui.message.scrollHeight > 144 ? "auto" : "hidden";
}

function syncMessageInsets() {
  const bottom = ui.messageForm.offsetHeight + 42;
  ui.messages.style.paddingBottom = `${bottom}px`;
  ui.messages.style.scrollPaddingBottom = `${bottom}px`;
}

ui.messageForm.addEventListener("submit", (event) => {
  event.preventDefault();
  sendMessage();
});
ui.message.addEventListener("input", resizeComposer);
ui.message.addEventListener("keydown", (event) => {
  if (event.key === "Enter" && (event.ctrlKey || event.metaKey)) {
    event.preventDefault();
    sendMessage();
  }
});
ui.refreshChanges.addEventListener("click", loadChanges);
ui.publishForm.addEventListener("submit", publish);
initializeInspector(loadChanges, loadEnvironment);
resizeComposer();
syncMessageInsets();
if (window.ResizeObserver) new ResizeObserver(syncMessageInsets).observe(ui.messageForm);
