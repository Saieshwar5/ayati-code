import { createServer } from "node:http";
import { readFile, writeFile, mkdtemp, rm } from "node:fs/promises";
import { spawn } from "node:child_process";
import { extname, join, normalize, resolve } from "node:path";
import { tmpdir } from "node:os";

const [, , kind, rootArg, outputArg] = process.argv;
if (!kind || !rootArg || !outputArg || !["simple", "complex"].includes(kind)) {
  console.error("usage: node browser-eval.mjs <simple|complex> <site-root> <output-prefix>");
  process.exit(2);
}

const siteRoot = resolve(rootArg);
const outputPrefix = resolve(outputArg);
const results = [];
const pageErrors = [];

function check(name, passed, detail = "") {
  results.push({ name, passed: Boolean(passed), detail: String(detail) });
}

const types = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".svg": "image/svg+xml",
};

const server = createServer(async (req, res) => {
  try {
    const pathname = new URL(req.url, "http://localhost").pathname;
    const relative = pathname === "/" ? "index.html" : pathname.slice(1);
    const path = resolve(siteRoot, normalize(relative));
    if (path !== siteRoot && !path.startsWith(siteRoot + "/")) throw new Error("outside root");
    const body = await readFile(path);
    res.writeHead(200, { "content-type": types[extname(path)] || "application/octet-stream" });
    res.end(body);
  } catch {
    res.writeHead(404);
    res.end("not found");
  }
});

await new Promise((resolveListen, reject) => {
  server.once("error", reject);
  server.listen(0, "127.0.0.1", resolveListen);
});
const port = server.address().port;

const profile = await mkdtemp(join(tmpdir(), "agent-eval-chrome-"));
const chrome = spawn("chromium", [
  "--headless=new",
  "--no-sandbox",
  "--disable-gpu",
  "--disable-background-networking",
  "--disable-component-update",
  "--disable-default-apps",
  "--disable-extensions",
  "--no-first-run",
  "--remote-debugging-port=0",
  `--user-data-dir=${profile}`,
  "about:blank",
], { stdio: ["ignore", "ignore", "pipe"] });

let stderr = "";
chrome.stderr.on("data", (chunk) => { stderr += chunk.toString(); });

async function waitForPort() {
  for (let i = 0; i < 100; i++) {
    try {
      const text = await readFile(join(profile, "DevToolsActivePort"), "utf8");
      return Number(text.split("\n")[0]);
    } catch {
      await new Promise((resolveWait) => setTimeout(resolveWait, 50));
    }
  }
  throw new Error(`Chromium did not expose DevToolsActivePort: ${stderr.slice(-1000)}`);
}

const debugPort = await waitForPort();
const pages = await fetch(`http://127.0.0.1:${debugPort}/json/list`).then((r) => r.json());
const pageTarget = pages.find((page) => page.type === "page" && !page.url.startsWith("chrome-extension://"));
if (!pageTarget) throw new Error(`No browser page target found: ${JSON.stringify(pages)}`);
const ws = new WebSocket(pageTarget.webSocketDebuggerUrl);
await new Promise((resolveOpen, reject) => {
  ws.addEventListener("open", resolveOpen, { once: true });
  ws.addEventListener("error", reject, { once: true });
});

let shuttingDown = false;
async function stopChrome() {
  if (chrome.exitCode !== null) return;
  await new Promise((resolveExit) => {
    const timer = setTimeout(resolveExit, 1500);
    chrome.once("exit", () => { clearTimeout(timer); resolveExit(); });
    chrome.kill("SIGTERM");
  });
}
async function fail(error) {
  if (shuttingDown) return;
  shuttingDown = true;
  console.error(error?.stack || error);
  try { ws.close(); } catch {}
  try { await stopChrome(); } catch {}
  try { server.close(); } catch {}
  try { await rm(profile, { recursive: true, force: true }); } catch {}
  process.exit(1);
}
process.on("uncaughtException", fail);
process.on("unhandledRejection", fail);

let nextId = 1;
const pending = new Map();
const eventWaiters = new Map();
ws.addEventListener("message", (event) => {
  const message = JSON.parse(event.data);
  if (message.id) {
    const waiter = pending.get(message.id);
    if (!waiter) return;
    pending.delete(message.id);
    if (message.error) waiter.reject(new Error(message.error.message));
    else waiter.resolve(message.result);
    return;
  }
  if (message.method === "Runtime.exceptionThrown") {
    pageErrors.push(message.params.exceptionDetails.text);
  }
  const waiters = eventWaiters.get(message.method);
  if (waiters?.length) waiters.shift()(message.params);
});

function cdp(method, params = {}) {
  const id = nextId++;
  return new Promise((resolveCall, reject) => {
    const timer = setTimeout(() => {
      pending.delete(id);
      reject(new Error(`CDP timeout: ${method}`));
    }, 15000);
    pending.set(id, {
      resolve: (value) => { clearTimeout(timer); resolveCall(value); },
      reject: (error) => { clearTimeout(timer); reject(error); },
    });
    ws.send(JSON.stringify({ id, method, params }));
  });
}

function waitEvent(method) {
  return new Promise((resolveEvent, reject) => {
    const list = eventWaiters.get(method) || [];
    const timer = setTimeout(() => reject(new Error(`CDP event timeout: ${method}`)), 15000);
    list.push((value) => { clearTimeout(timer); resolveEvent(value); });
    eventWaiters.set(method, list);
  });
}

async function evaluate(expression) {
  const result = await cdp("Runtime.evaluate", {
    expression,
    awaitPromise: true,
    returnByValue: true,
  });
  if (result.exceptionDetails) {
    const description = result.exceptionDetails.exception?.description || result.exceptionDetails.text;
    throw new Error(description);
  }
  return result.result.value;
}

async function navigate() {
  await cdp("Page.navigate", { url: `http://127.0.0.1:${port}/index.html` });
  await waitReady();
}

async function waitReady() {
  let last;
  for (let i = 0; i < 100; i++) {
    last = await evaluate(`({href: location.href, port: location.port, ready: document.readyState})`);
    if (last.port === String(port) && last.ready === "complete") {
      await new Promise((resolveWait) => setTimeout(resolveWait, 200));
      return;
    }
    await new Promise((resolveWait) => setTimeout(resolveWait, 50));
  }
  throw new Error(`Page did not reach the expected URL and readyState: ${JSON.stringify(last)}`);
}

async function screenshot(name, width, height) {
  await cdp("Emulation.setDeviceMetricsOverride", {
    width,
    height,
    deviceScaleFactor: 1,
    mobile: width < 600,
  });
  await new Promise((resolveWait) => setTimeout(resolveWait, 150));
  const capture = await cdp("Page.captureScreenshot", { format: "png", captureBeyondViewport: false });
  await writeFile(`${outputPrefix}-${name}.png`, Buffer.from(capture.data, "base64"));
}

await cdp("Page.enable");
await cdp("Runtime.enable");
await navigate();
await evaluate("localStorage.clear()");
await cdp("Page.reload", { ignoreCache: true });
await waitReady();

if (kind === "simple") {
  const presence = await evaluate(`(() => ({
    nav: !!document.querySelector('#nav-toggle'),
    theme: !!document.querySelector('#theme-toggle'),
    form: !!document.querySelector('#newsletter-form'),
    email: !!document.querySelector('#email'),
    status: !!document.querySelector('#form-status'),
    sections: document.querySelectorAll('main section').length,
    overflow: document.documentElement.scrollWidth <= document.documentElement.clientWidth
  }))()`);
  check("required controls", presence.nav && presence.theme && presence.form && presence.email && presence.status, JSON.stringify(presence));
  check("content sections", presence.sections >= 6, presence.sections);
  check("desktop no horizontal overflow", presence.overflow, JSON.stringify(presence));

  const forms = await evaluate(`(() => {
    const form = document.querySelector('#newsletter-form');
    const email = document.querySelector('#email');
    const status = document.querySelector('#form-status');
    email.value = 'invalid'; form.requestSubmit(); const invalid = status.textContent.trim();
    email.value = 'test@example.com'; form.requestSubmit(); const valid = status.textContent.trim();
    return { invalid, valid };
  })()`);
  check("invalid email rejected", forms.invalid.length > 0, forms.invalid);
  check("valid email accepted", forms.valid.length > 0 && forms.valid !== forms.invalid, forms.valid);

  const theme = await evaluate(`(() => {
    document.querySelector('#theme-toggle').click();
    return { value: localStorage.getItem('orbit-theme'), theme: document.documentElement.getAttribute('data-theme') };
  })()`);
  check("theme persisted", ["light", "dark"].includes(theme.value) && theme.theme === theme.value, JSON.stringify(theme));

  await cdp("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 1, mobile: true });
  const mobile = await evaluate(`(() => {
    const toggle = document.querySelector('#nav-toggle');
    toggle.click();
    const links = document.querySelector(toggle.getAttribute('aria-controls') ? '#' + toggle.getAttribute('aria-controls') : 'nav ul');
    return {
      expanded: toggle.getAttribute('aria-expanded'),
      visible: links ? getComputedStyle(links).display !== 'none' : false,
      overflow: document.documentElement.scrollWidth <= document.documentElement.clientWidth
    };
  })()`);
  check("mobile navigation opens", mobile.expanded === "true" && mobile.visible, JSON.stringify(mobile));
  check("mobile no horizontal overflow", mobile.overflow, JSON.stringify(mobile));
} else {
  const initial = await evaluate(`(() => ({
    rows: document.querySelectorAll('.transaction-row').length,
    controls: ['transaction-form','description','amount','type','category','date','form-error','search','type-filter','category-filter','sort-by','budget-input','save-budget'].every(id => !!document.getElementById(id)),
    stored: JSON.parse(localStorage.getItem('ledgerly-transactions') || '[]').length,
    overflow: document.documentElement.scrollWidth <= document.documentElement.clientWidth
  }))()`);
  check("required controls", initial.controls, JSON.stringify(initial));
  check("seeded transactions", initial.rows >= 5 && initial.stored >= 5, JSON.stringify(initial));
  check("desktop no horizontal overflow", initial.overflow, JSON.stringify(initial));

  const validation = await evaluate(`(() => {
    const form = document.querySelector('#transaction-form');
    const error = document.querySelector('#form-error');
    const d = document.querySelector('#description');
    const a = document.querySelector('#amount');
    const date = document.querySelector('#date');
    d.value=''; a.value='10'; date.value='2026-08-08'; form.requestSubmit(); const empty = error.textContent.trim();
    d.value='Test'; a.value='0'; date.value='2026-08-08'; form.requestSubmit(); const amount = error.textContent.trim();
    d.value='Test'; a.value='10'; date.value=''; form.requestSubmit(); const missingDate = error.textContent.trim();
    return { empty, amount, missingDate };
  })()`);
  check("description validation", validation.empty.length > 0, validation.empty);
  check("amount validation", validation.amount.length > 0, validation.amount);
  check("date validation", validation.missingDate.length > 0, validation.missingDate);

  const addition = await evaluate(`(() => {
    const before = document.querySelectorAll('.transaction-row').length;
    document.querySelector('#description').value='Evaluation coffee';
    document.querySelector('#amount').value='12.50';
    document.querySelector('#type').value='expense';
    const category = document.querySelector('#category'); category.selectedIndex = Math.min(1, category.options.length - 1);
    document.querySelector('#date').value='2026-08-08';
    document.querySelector('#transaction-form').requestSubmit();
    return { before, after: document.querySelectorAll('.transaction-row').length, stored: JSON.parse(localStorage.getItem('ledgerly-transactions') || '[]').length };
  })()`);
  check("add transaction and persist", addition.after === addition.before + 1 && addition.stored === addition.after, JSON.stringify(addition));

  const filters = await evaluate(`(() => {
    const search = document.querySelector('#search'); search.value='Evaluation coffee'; search.dispatchEvent(new Event('input', {bubbles:true}));
    const type = document.querySelector('#type-filter'); type.value='expense'; type.dispatchEvent(new Event('change', {bubbles:true}));
    return { rows: document.querySelectorAll('.transaction-row').length, text: document.body.innerText.includes('Evaluation coffee') };
  })()`);
  check("combined filters", filters.rows === 1 && filters.text, JSON.stringify(filters));

  const budget = await evaluate(`(() => {
    const input = document.querySelector('#budget-input'); const button = document.querySelector('#save-budget');
    input.value='0'; button.click(); const zeroText = document.body.innerText; const zeroStored = localStorage.getItem('ledgerly-budget');
    input.value='1'; button.click();
    const progress = document.querySelector('[role="progressbar"], progress, .budget-progress, #budget-progress');
    return { zeroStored, safe: !/NaN|Infinity/.test(zeroText), overText: document.body.innerText, progress: progress ? (progress.getAttribute('aria-valuenow') || progress.value || getComputedStyle(progress).width) : '' };
  })()`);
  check("zero budget safe and persisted", budget.safe && budget.zeroStored !== null, JSON.stringify(budget));
  check("overspending represented", /over|exceed|spent|used/i.test(budget.overText), String(budget.progress));

  const theme = await evaluate(`(() => {
    const button = document.querySelector('#theme-toggle, [data-theme-toggle]');
    if (!button) return { present:false };
    button.click();
    return { present:true, value:localStorage.getItem('ledgerly-theme'), theme:document.documentElement.getAttribute('data-theme') };
  })()`);
  let storedTheme = theme.value;
  try { storedTheme = JSON.parse(storedTheme); } catch {}
  check("theme persisted", theme.present && ["light", "dark"].includes(storedTheme) && theme.theme === storedTheme, JSON.stringify(theme));

  const deletion = await evaluate(`(() => {
    document.querySelector('#reset-filters')?.click();
    const before = document.querySelectorAll('.transaction-row').length;
    document.querySelector('.delete-transaction')?.click();
    const after = document.querySelectorAll('.transaction-row').length;
    const stored = JSON.parse(localStorage.getItem('ledgerly-transactions') || '[]').length;
    return { before, after, stored };
  })()`);
  check("delete transaction and persist", deletion.after === deletion.before - 1 && deletion.stored === deletion.after, JSON.stringify(deletion));

  const emptyState = await evaluate(`(() => {
    const element = document.querySelector('#empty-state');
    return element ? { hidden: element.hidden, display: getComputedStyle(element).display } : { missing: true };
  })()`);
  check("empty state visually hidden with populated rows", emptyState.hidden && emptyState.display === "none", JSON.stringify(emptyState));

  await cdp("Emulation.setDeviceMetricsOverride", { width: 390, height: 844, deviceScaleFactor: 1, mobile: true });
  const mobile = await evaluate(`(() => ({
    overflow: document.documentElement.scrollWidth <= document.documentElement.clientWidth,
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth
  }))()`);
  check("mobile no horizontal overflow", mobile.overflow, JSON.stringify(mobile));
}

await screenshot("desktop", 1440, 900);
await screenshot("mobile", 390, 844);
check("no uncaught page exceptions", pageErrors.length === 0, pageErrors.join("; "));

const summary = {
  kind,
  siteRoot,
  passed: results.filter((r) => r.passed).length,
  failed: results.filter((r) => !r.passed).length,
  results,
};
await writeFile(`${outputPrefix}.json`, JSON.stringify(summary, null, 2) + "\n");
console.log(JSON.stringify(summary, null, 2));

ws.close();
server.close();
await stopChrome();
await rm(profile, { recursive: true, force: true });
shuttingDown = true;
process.exitCode = summary.failed ? 1 : 0;
