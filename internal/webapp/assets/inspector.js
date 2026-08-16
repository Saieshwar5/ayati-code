import { ui } from "./shared.js";

export function renderActivityMessage(message) {
  if (message.role === "assistant" && message.tool_calls?.length) {
    for (const call of message.tool_calls) renderToolCall(call);
    return true;
  }
  if (message.role === "tool") {
    renderToolResult(message);
    return true;
  }
  return false;
}

export function renderActivityEmpty() {
  const empty = document.createElement("p");
  empty.className = "activity-empty";
  empty.textContent = "Shell commands, results, and verification from this session will appear here.";
  ui.activity.append(empty);
}

function renderToolCall(call) {
  const entry = document.createElement("article");
  entry.className = "activity-entry command";
  let command = call.function?.arguments || "";
  try { command = JSON.parse(command).command; } catch (_) { /* show raw arguments */ }
  const heading = document.createElement("div");
  heading.className = "activity-entry-heading";
  heading.innerHTML = "<span>Shell command</span><span>shell</span>";
  const output = document.createElement("pre");
  output.textContent = `$ ${command}`;
  entry.append(heading, output);
  ui.activity.append(entry);
}

function renderToolResult(message) {
  let result;
  try { result = JSON.parse(message.content); } catch (_) { result = { stdout: message.content, exit_code: 1 }; }
  const entry = document.createElement("details");
  const failed = result.exit_code !== 0 || result.error || result.timed_out;
  entry.className = `activity-entry result${failed ? " failed" : ""}`;
  const outputText = [result.stdout, result.stderr, result.error].filter(Boolean).join("\n") || "No output.";
  entry.open = failed || outputText.length < 500;
  const heading = document.createElement("summary");
  heading.className = "activity-entry-heading";
  const duration = result.duration ? ` · ${formatDuration(result.duration)}` : "";
  heading.innerHTML = `<span>Command result</span><span>exit ${result.exit_code}${duration}</span>`;
  const output = document.createElement("pre");
  output.textContent = outputText;
  entry.append(heading, output);
  ui.activity.append(entry);
}

function formatDuration(nanoseconds) {
  const milliseconds = Number(nanoseconds) / 1e6;
  return milliseconds >= 1000 ? `${(milliseconds / 1000).toFixed(1)}s` : `${Math.round(milliseconds)}ms`;
}

export function selectInspectorPanel(name, loadChanges) {
  ui.inspector.querySelector(".inspector-title h2").textContent = name[0].toUpperCase() + name.slice(1);
  document.querySelectorAll(".inspector-tab").forEach((tab) => {
    const selected = tab.dataset.panel === name;
    tab.classList.toggle("active", selected);
    tab.setAttribute("aria-selected", String(selected));
  });
  for (const panel of [ui.activityPanel, ui.changesPanel, ui.publishPanel]) {
    panel.classList.toggle("active", panel.id === `${name}-panel`);
  }
  if (name === "changes") loadChanges();
}

export function setInspectorCollapsed(collapsed) {
  ui.dashboard.classList.toggle("inspector-collapsed", collapsed);
  ui.inspectorToggle.textContent = collapsed ? "‹" : "›";
  ui.inspectorToggle.setAttribute("aria-expanded", String(!collapsed));
  ui.inspectorToggle.setAttribute("aria-label", collapsed ? "Open internal work" : "Collapse internal work");
  try { localStorage.setItem("ayati.inspector.collapsed", String(collapsed)); } catch (_) { /* optional preference */ }
}

export function initializeInspector(loadChanges) {
  ui.inspectorToggle.addEventListener("click", () => {
    setInspectorCollapsed(!ui.dashboard.classList.contains("inspector-collapsed"));
  });
  document.querySelectorAll(".inspector-tab").forEach((tab) => {
    tab.addEventListener("click", () => selectInspectorPanel(tab.dataset.panel, loadChanges));
  });
  let collapsed = window.matchMedia("(max-width: 880px)").matches;
  try {
    const saved = localStorage.getItem("ayati.inspector.collapsed");
    if (saved !== null) collapsed = saved === "true";
  } catch (_) { /* optional preference */ }
  setInspectorCollapsed(collapsed);
}
