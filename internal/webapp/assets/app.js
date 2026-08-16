import { api, option, show, state, ui } from "./shared.js";
import { loadWorkspaces } from "./navigation.js";

async function start() {
  try {
    const session = await api("/api/session");
    if (!session.github_configured) return show(ui.configure);
    if (!session.authenticated) return show(ui.login);
    renderAccount(session.user);
    show(ui.dashboard);
    const [repositories] = await Promise.allSettled([loadRepositories(), loadWorkspaces({ openFirst: true })]);
    if (repositories.status === "rejected") showRepositoryError(repositories.reason.message);
  } catch (error) {
    show(ui.loading);
    ui.loading.querySelector("h1").textContent = error.message;
  }
}

function renderAccount(user) {
  const avatar = document.createElement("img");
  avatar.className = "avatar";
  avatar.alt = "";
  avatar.src = user.avatar_url;
  const name = document.createElement("span");
  name.textContent = user.login;
  const logout = document.createElement("button");
  logout.className = "quiet";
  logout.textContent = "Sign out";
  logout.addEventListener("click", async () => {
    await api("/api/logout", { method: "POST" });
    location.reload();
  });
  ui.account.replaceChildren(avatar, name, logout);
}

async function loadRepositories() {
  state.repositories = await api("/api/repositories");
  ui.repository.disabled = state.repositories.length === 0;
  const label = state.repositories.length === 0 ? "No installed repositories" : "Select a repository";
  ui.repository.replaceChildren(option("", label));
  for (const repository of state.repositories) {
    ui.repository.append(option(repository.full_name, repository.full_name));
  }
}

function showRepositoryError(message) {
  ui.repository.disabled = true;
  ui.repository.replaceChildren(option("", "Repositories unavailable"));
  ui.workspaceEmpty.querySelector(".muted").textContent = `${message}. Check the GitHub App installation, then reload Ayati.`;
  showFormError(message);
}

async function loadBranches() {
  const repository = ui.repository.value;
  ui.baseBranch.disabled = true;
  if (!repository) {
    ui.baseBranch.replaceChildren(option("", "Select a repository first"));
    ui.existingBranch.replaceChildren(option("", "Select a repository first"));
    return;
  }
  ui.baseBranch.replaceChildren(option("", "Loading branches…"));
  try {
    const branches = await api(`/api/repositories/${repository}/branches`);
    ui.baseBranch.replaceChildren(option("", "Select a branch"));
    ui.existingBranch.replaceChildren(option("", "Select a branch"));
    for (const branch of branches) {
      ui.baseBranch.append(option(branch.name, branch.name));
      ui.existingBranch.append(option(branch.name, branch.name));
    }
    const selected = state.repositories.find((item) => item.full_name === repository)?.default_branch;
    if (selected) ui.baseBranch.value = selected;
    syncBranch();
  } catch (error) {
    showFormError(error.message);
  } finally {
    ui.baseBranch.disabled = false;
  }
}

function syncBranch() {
  const create = ui.createBranch.checked;
  ui.newBranchField.classList.toggle("hidden", !create);
  ui.existingBranchField.classList.toggle("hidden", create);
  ui.branch.disabled = !create;
  ui.branch.required = create;
  ui.existingBranch.disabled = create;
  ui.existingBranch.required = !create;
}

function showFormError(message = "") {
  ui.formError.textContent = message;
  ui.formError.classList.toggle("hidden", !message);
}

function showCreateWorkspace() {
  state.activeWorkspace = null;
  state.activeSession = null;
  document.querySelectorAll(".workspace-item.active").forEach((item) => item.classList.remove("active"));
  ui.detail.classList.add("hidden");
  ui.home.classList.remove("hidden");
  ui.workspaceEmpty.classList.add("hidden");
  ui.form.classList.remove("hidden");
  ui.inspectorEmpty.classList.remove("hidden");
  ui.inspectorContent.classList.add("hidden");
  showFormError();
  ui.repository.focus();
}

function closeCreateWorkspace() {
  ui.form.classList.add("hidden");
  ui.workspaceEmpty.classList.remove("hidden");
}

ui.showCreate.addEventListener("click", showCreateWorkspace);
ui.emptyCreate.addEventListener("click", showCreateWorkspace);
ui.cancelCreate.addEventListener("click", closeCreateWorkspace);
ui.repository.addEventListener("change", loadBranches);
ui.baseBranch.addEventListener("change", syncBranch);
ui.createBranch.addEventListener("change", syncBranch);
ui.form.addEventListener("submit", async (event) => {
  event.preventDefault();
  showFormError();
  const submit = ui.form.querySelector("button[type=submit]");
  submit.disabled = true;
  try {
    const created = await api("/api/workspaces", {
      method: "POST",
      body: JSON.stringify({
        repository: ui.repository.value,
        base_branch: ui.baseBranch.value,
        branch: ui.createBranch.checked ? ui.branch.value : ui.existingBranch.value,
        create_branch: ui.createBranch.checked,
        setup_command: ui.setup.value,
      }),
    });
    ui.form.reset();
    syncBranch();
    await loadWorkspaces({ selectWorkspaceID: created.id });
  } catch (error) {
    showFormError(error.message);
  } finally {
    submit.disabled = false;
  }
});

syncBranch();
start();
