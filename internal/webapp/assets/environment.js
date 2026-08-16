import { api, state, ui } from "./shared.js";

function createDraft() {
  const row = document.createElement("div");
  row.className = "environment-draft";
  row.innerHTML = `
    <label>Name<input class="environment-draft-name" autocomplete="off" placeholder="DATABASE_URL" required></label>
    <label>Value<input class="environment-draft-value" type="password" autocomplete="new-password" placeholder="Secret or configuration" required></label>
    <label class="check-row environment-draft-setup"><input type="checkbox">During setup</label>
    <button class="quiet compact" type="button" aria-label="Remove variable">Remove</button>`;
  row.querySelector("button").addEventListener("click", () => row.remove());
  ui.createEnvironmentList.append(row);
  row.querySelector("input").focus();
}

export function creationEnvironment() {
  return [...ui.createEnvironmentList.querySelectorAll(".environment-draft")].map((row) => ({
    name: row.querySelector(".environment-draft-name").value.trim(),
    value: row.querySelector(".environment-draft-value").value,
    expose_during_setup: row.querySelector(".environment-draft-setup input").checked,
  }));
}

export function resetCreationEnvironment() {
  ui.createEnvironmentList.replaceChildren();
}

function environmentLocked() {
  if (!state.activeWorkspace) return true;
  const working = (state.sessions[state.activeWorkspace.id] || []).some((session) => session.status === "working");
  return working || ["creating", "initializing"].includes(state.activeWorkspace.status);
}

function showEnvironmentError(message = "") {
  ui.environmentError.textContent = message;
  ui.environmentError.classList.toggle("hidden", !message);
}

function resetEnvironmentForm() {
  ui.environmentForm.reset();
  ui.environmentName.disabled = false;
  ui.environmentForm.querySelector("button[type=submit]").textContent = "Add variable";
  ui.cancelEnvironmentEdit.classList.add("hidden");
  showEnvironmentError();
}

function editEnvironment(variable) {
  ui.environmentName.value = variable.name;
  ui.environmentName.disabled = true;
  ui.environmentValue.value = "";
  ui.environmentSetup.checked = variable.expose_during_setup;
  ui.environmentForm.querySelector("button[type=submit]").textContent = "Replace value";
  ui.cancelEnvironmentEdit.classList.remove("hidden");
  ui.environmentValue.focus();
}

function renderEnvironment(values) {
  ui.environmentList.replaceChildren();
  if (!values.length) {
    const empty = document.createElement("p");
    empty.className = "environment-empty";
    empty.textContent = "No workspace variables configured.";
    ui.environmentList.append(empty);
  }
  for (const variable of values) {
    const row = document.createElement("article");
    row.className = "environment-variable";
    const identity = document.createElement("div");
    const name = document.createElement("code");
    name.textContent = variable.name;
    const scope = document.createElement("span");
    scope.textContent = variable.expose_during_setup ? "Setup + development" : "Development";
    identity.append(name, scope);
    const actions = document.createElement("div");
    const replace = document.createElement("button");
    replace.className = "quiet compact";
    replace.type = "button";
    replace.textContent = "Replace";
    replace.addEventListener("click", () => editEnvironment(variable));
    const remove = document.createElement("button");
    remove.className = "quiet compact danger-text";
    remove.type = "button";
    remove.textContent = "Delete";
    remove.addEventListener("click", () => deleteEnvironment(variable.name));
    actions.append(replace, remove);
    row.append(identity, actions);
    ui.environmentList.append(row);
  }
  syncEnvironmentAvailability();
}

export function syncEnvironmentAvailability() {
  const locked = environmentLocked();
  ui.environmentList.querySelectorAll("button").forEach((button) => { button.disabled = locked; });
  for (const control of ui.environmentForm.elements) control.disabled = locked;
  if (!locked) ui.environmentName.disabled = !ui.cancelEnvironmentEdit.classList.contains("hidden");
}

export async function loadEnvironment() {
  if (!state.activeWorkspace) return;
  const workspaceID = state.activeWorkspace.id;
  try {
    const values = await api(`/api/workspaces/${workspaceID}/environment`);
    if (state.activeWorkspace?.id === workspaceID) renderEnvironment(values);
  } catch (error) {
    if (state.activeWorkspace?.id === workspaceID) showEnvironmentError(error.message);
  }
}

async function deleteEnvironment(name) {
  if (!state.activeWorkspace || !confirm(`Delete ${name} from this workspace?`)) return;
  try {
    await api(`/api/workspaces/${state.activeWorkspace.id}/environment/${encodeURIComponent(name)}`, { method: "DELETE" });
    resetEnvironmentForm();
    await loadEnvironment();
  } catch (error) {
    showEnvironmentError(error.message);
  }
}

ui.addCreateEnvironment.addEventListener("click", createDraft);
ui.cancelEnvironmentEdit.addEventListener("click", resetEnvironmentForm);
ui.environmentForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  if (!state.activeWorkspace) return;
  showEnvironmentError();
  const submit = ui.environmentForm.querySelector("button[type=submit]");
  submit.disabled = true;
  try {
    await api(`/api/workspaces/${state.activeWorkspace.id}/environment`, {
      method: "POST",
      body: JSON.stringify({
        name: ui.environmentName.value,
        value: ui.environmentValue.value,
        expose_during_setup: ui.environmentSetup.checked,
      }),
    });
    resetEnvironmentForm();
    await loadEnvironment();
  } catch (error) {
    showEnvironmentError(error.message);
  } finally {
    submit.disabled = false;
  }
});
