import { api, state, ui } from "./shared.js";
import { openWorkspace } from "./workspace.js";

const refreshingStatuses = new Set(["creating", "initializing"]);

export async function loadWorkspaces({ selectWorkspaceID = "", openFirst = false } = {}) {
  state.workspaces = await api("/api/workspaces");
  ui.list.replaceChildren();
  ui.navEmpty.classList.toggle("hidden", state.workspaces.length !== 0);
  let refreshing = false;
  for (const workspace of state.workspaces) {
    ui.list.append(renderWorkspace(workspace));
    if (refreshingStatuses.has(workspace.status)) refreshing = true;
  }
  const desired = selectWorkspaceID || state.activeWorkspace?.id || (openFirst ? state.workspaces[0]?.id : "");
  if (desired) await expandWorkspace(desired, true);
  clearTimeout(state.refreshTimer);
  if (refreshing) state.refreshTimer = setTimeout(() => loadWorkspaces(), 1200);
}

function renderWorkspace(workspace) {
  const item = ui.template.content.firstElementChild.cloneNode(true);
  item.dataset.workspaceId = workspace.id;
  item.classList.add(workspace.status);
  const repository = item.querySelector(".workspace-repository");
  repository.textContent = workspace.repository.split("/").pop();
  item.querySelector(".workspace-branch").textContent = workspace.branch;
  const status = item.querySelector(".workspace-status");
  status.textContent = workspace.status.replaceAll("_", " ");
  status.classList.add(workspace.status);
  const open = item.querySelector(".workspace-open");
  open.title = `${workspace.repository} · ${workspace.branch}`;
  open.addEventListener("click", () => toggleWorkspace(workspace.id));

  for (const button of item.querySelectorAll(".new-session, .inline-new-session")) {
    button.addEventListener("click", () => createSession(workspace.id));
  }
  const retry = item.querySelector(".retry");
  if (["initialization_failed", "stopped"].includes(workspace.status)) {
    retry.classList.remove("hidden");
    retry.addEventListener("click", () => workspaceAction(workspace.id, "initialize"));
  }
  const stop = item.querySelector(".stop");
  if (["ready", "initialization_failed"].includes(workspace.status)) {
    stop.classList.remove("hidden");
    stop.addEventListener("click", () => workspaceAction(workspace.id, "stop"));
  }
  const pull = item.querySelector(".workspace-pull");
  if (workspace.pull_request_url) {
    pull.classList.remove("hidden");
    pull.href = workspace.pull_request_url;
  }
  const remove = item.querySelector(".delete-workspace");
  if (!refreshingStatuses.has(workspace.status)) {
    remove.classList.remove("hidden");
    remove.addEventListener("click", () => deleteWorkspace(workspace));
  }
  return item;
}

async function toggleWorkspace(workspaceID) {
  if (state.expandedWorkspaceID === workspaceID) {
    collapseWorkspace(workspaceID);
    return;
  }
  await expandWorkspace(workspaceID, true);
}

async function expandWorkspace(workspaceID, selectSession) {
  document.querySelectorAll(".workspace-item").forEach((item) => {
    const expanded = item.dataset.workspaceId === workspaceID;
    item.classList.toggle("expanded", expanded);
    item.querySelector(".session-navigation").classList.toggle("hidden", !expanded);
    item.querySelector(".workspace-open").setAttribute("aria-expanded", String(expanded));
  });
  state.expandedWorkspaceID = workspaceID;
  const sessions = await loadSessions(workspaceID);
  if (!selectSession || sessions.length === 0) return;
  const active = state.activeSession?.workspace_id === workspaceID ? state.activeSession : sessions[0];
  await openWorkspace(workspaceID, active.id);
}

function collapseWorkspace(workspaceID) {
  const item = workspaceElement(workspaceID);
  if (!item) return;
  item.classList.remove("expanded");
  item.querySelector(".session-navigation").classList.add("hidden");
  item.querySelector(".workspace-open").setAttribute("aria-expanded", "false");
  state.expandedWorkspaceID = null;
}

async function loadSessions(workspaceID) {
  const sessions = await api(`/api/workspaces/${workspaceID}/sessions`);
  state.sessions[workspaceID] = sessions;
  renderSessions(workspaceID);
  return sessions;
}

function renderSessions(workspaceID) {
  const item = workspaceElement(workspaceID);
  if (!item) return;
  const list = item.querySelector(".session-list");
  list.replaceChildren();
  for (const session of state.sessions[workspaceID] || []) {
    const row = ui.sessionTemplate.content.firstElementChild.cloneNode(true);
    row.dataset.sessionId = session.id;
    row.classList.add(session.status);
    row.classList.toggle("active", state.activeSession?.id === session.id);
    row.querySelector(".session-title").textContent = session.title;
    row.querySelector(".session-meta").textContent = sessionMeta(session);
    row.querySelector(".session-open").addEventListener("click", () => openWorkspace(workspaceID, session.id));
    row.querySelector(".rename-session").addEventListener("click", () => renameSession(workspaceID, session));
    row.querySelector(".delete-session").addEventListener("click", () => deleteSession(workspaceID, session));
    list.append(row);
  }
}

async function createSession(workspaceID) {
  const session = await api(`/api/workspaces/${workspaceID}/sessions`, {
    method: "POST", body: JSON.stringify({}),
  });
  await loadSessions(workspaceID);
  await openWorkspace(workspaceID, session.id);
}

async function renameSession(workspaceID, session) {
  const title = window.prompt("Rename session", session.title)?.trim();
  if (!title || title === session.title) return;
  await api(`/api/workspaces/${workspaceID}/sessions/${session.id}`, {
    method: "PATCH", body: JSON.stringify({ title }),
  });
  await loadSessions(workspaceID);
  if (state.activeSession?.id === session.id) {
    state.activeSession.title = title;
    ui.detailSessionTitle.textContent = title;
  }
}

async function deleteSession(workspaceID, session) {
  const confirmed = window.confirm(
    `Delete “${session.title}”?\n\nThis removes its conversation and activity history. Workspace files and changes are not reverted.`,
  );
  if (!confirmed) return;
  try {
    await api(`/api/workspaces/${workspaceID}/sessions/${session.id}`, { method: "DELETE" });
    const sessions = await loadSessions(workspaceID);
    if (state.activeSession?.id === session.id && sessions.length) await openWorkspace(workspaceID, sessions[0].id);
  } catch (error) {
    window.alert(error.message);
  }
}

async function deleteWorkspace(workspace) {
  const name = workspace.repository.split("/").pop();
  const confirmed = window.confirm(
    `Delete workspace “${name}”?\n\nThis permanently removes its local clone, every session, and all conversation and activity history. The GitHub branch and pull request are not deleted.`,
  );
  if (!confirmed) return;
  try {
    await api(`/api/workspaces/${workspace.id}`, { method: "DELETE" });
    delete state.sessions[workspace.id];
    const wasActive = state.activeWorkspace?.id === workspace.id;
    if (state.expandedWorkspaceID === workspace.id) state.expandedWorkspaceID = null;
    if (wasActive) resetWorkspaceView();
    await loadWorkspaces({ openFirst: wasActive });
  } catch (error) {
    window.alert(error.message);
  }
}

function resetWorkspaceView() {
  state.activeWorkspace = null;
  state.activeSession = null;
  clearInterval(state.messagePollTimer);
  state.messagePollTimer = null;
  state.messagePollBusy = false;
  ui.detail.classList.add("hidden");
  ui.home.classList.remove("hidden");
  ui.form.classList.add("hidden");
  ui.workspaceEmpty.classList.remove("hidden");
  ui.inspectorContent.classList.add("hidden");
  ui.inspectorEmpty.classList.remove("hidden");
}

async function workspaceAction(id, action) {
  try {
    await api(`/api/workspaces/${id}/${action}`, { method: "POST" });
    await loadWorkspaces({ selectWorkspaceID: id });
  } catch (error) {
    window.alert(error.message);
  }
}

function workspaceElement(id) {
  return document.querySelector(`[data-workspace-id="${CSS.escape(id)}"]`);
}

function sessionMeta(session) {
  if (session.status === "working") return "Working now";
  if (session.status === "failed") return "Failed";
  if (session.status === "review") return "Review changes";
  const time = new Date(session.updated_at);
  const elapsed = Date.now() - time.getTime();
  if (elapsed < 60_000) return "Just now";
  if (elapsed < 3_600_000) return `${Math.floor(elapsed / 60_000)}m ago`;
  if (elapsed < 86_400_000) return `${Math.floor(elapsed / 3_600_000)}h ago`;
  return time.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

window.addEventListener("ayati:session-updated", (event) => {
  const session = event.detail;
  const sessions = state.sessions[session.workspace_id] || [];
  const index = sessions.findIndex((item) => item.id === session.id);
  if (index >= 0) sessions[index] = session;
  sessions.sort((left, right) => new Date(right.updated_at) - new Date(left.updated_at));
  renderSessions(session.workspace_id);
});
