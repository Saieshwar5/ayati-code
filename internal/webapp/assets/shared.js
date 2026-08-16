export const ui = {};

for (const [name, selector] of Object.entries({
  loading: "#loading", configure: "#configure", login: "#login", dashboard: "#dashboard",
  account: "#account", showCreate: "#show-create", emptyCreate: "#empty-create",
  cancelCreate: "#cancel-create", form: "#create-form", repository: "#repository",
  baseBranch: "#base-branch", createBranch: "#create-branch", branch: "#branch", setup: "#setup",
  newBranchField: "#new-branch-field", existingBranchField: "#existing-branch-field",
  existingBranch: "#existing-branch",
  addCreateEnvironment: "#add-create-environment", createEnvironmentList: "#create-environment-list",
  formError: "#form-error", list: "#workspace-list", navEmpty: "#workspace-nav-empty",
  template: "#workspace-template", sessionTemplate: "#session-template",
  home: "#workspace-home", workspaceEmpty: "#workspace-empty",
  detail: "#workspace-detail", detailRepository: "#detail-repository", detailBranch: "#detail-branch",
  detailSessionTitle: "#detail-session-title", detailStatus: "#detail-status",
  messages: "#messages", messageForm: "#message-form",
  message: "#message", messageError: "#message-error", sendMessage: "#send-message",
  refreshChanges: "#refresh-changes", changes: "#changes",
  pullLink: "#pull-link", publishForm: "#publish-form", commitMessage: "#commit-message",
  pullTitle: "#pull-title", pullBody: "#pull-body", publishError: "#publish-error",
  detailError: "#detail-error", inspector: "#inspector", inspectorToggle: "#inspector-toggle",
  inspectorEmpty: "#inspector-empty", inspectorContent: "#inspector-content",
  activityPanel: "#activity-panel", changesPanel: "#changes-panel", publishPanel: "#publish-panel",
  environmentPanel: "#environment-panel", environmentList: "#environment-list",
  environmentForm: "#environment-form", environmentName: "#environment-name",
  environmentValue: "#environment-value", environmentSetup: "#environment-setup",
  environmentError: "#environment-error", cancelEnvironmentEdit: "#cancel-environment-edit",
  activityState: "#activity-state", activity: "#activity",
})) ui[name] = document.querySelector(selector);

export const state = {
  repositories: [],
  workspaces: [],
  sessions: {},
  activeWorkspace: null,
  activeSession: null,
  expandedWorkspaceID: null,
  refreshTimer: null,
  messagePollTimer: null,
  messagePollBusy: false,
};

export async function api(path, options = {}) {
  const headers = { ...(options.headers || {}) };
  if (options.body) headers["Content-Type"] = "application/json";
  if (options.method && options.method !== "GET") headers["X-Ayati-Request"] = "1";
  const response = await fetch(path, { ...options, headers });
  if (response.status === 204 || (response.status === 202 && !response.headers.get("content-type"))) return null;
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error || `Request failed (${response.status})`);
  return body;
}

export function show(section) {
  [ui.loading, ui.configure, ui.login, ui.dashboard].forEach((item) => item.classList.add("hidden"));
  section.classList.remove("hidden");
}

export function option(value, label) {
  const element = document.createElement("option");
  element.value = value;
  element.textContent = label;
  return element;
}
