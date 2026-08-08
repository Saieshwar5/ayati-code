'use strict';
/*
 * Ledgerly — end-to-end browser audit.
 * Drives a real (headless) Chromium over the CDP protocol using only Node
 * built-ins. Covers interactions, persistence, accessibility, and
 * responsive-layout properties.
 *
 * Prereqs: chromium binary + a static server.
 *   python3 -m http.server 8765 &
 *   node tests/e2e.js            (chromium auto-discovered, or CHROME_BIN=...)
 */
const assert = require('assert');
const { spawn, execSync } = require('child_process');
const http = require('http');
const fs = require('fs');
const path = require('path');

/* A per-invocation port avoids two test runs racing on the same CDP endpoint. */
const PORT = process.env.CDP_PORT || (9344 + Math.floor(Math.random() * 200));
const APP = process.env.APP_URL || 'http://localhost:8765/index.html';

let CHROME = process.env.CHROME_BIN || '';
if (!CHROME) {
  const candidates = ['chromium', 'chromium-browser', 'google-chrome', '/usr/bin/chromium'];
  for (const c of candidates) {
    try { execSync(`command -v ${c} 2>/dev/null`); CHROME = c; break; } catch (e) {}
  }
}
if (!CHROME) { console.error('no chromium found; set CHROME_BIN'); process.exit(2); }

function getJson(url) {
  return new Promise((resolve, reject) => {
    http.get(url, (r) => { let d = ''; r.on('data', (c) => (d += c)); r.on('end', () => { try { resolve(JSON.parse(d)); } catch (e) { reject(e); } }); }).on('error', reject);
  });
}

let failures = 0;
async function test(name, fn, ctx) {
  try { await fn(ctx); console.log('  PASS  ' + name); }
  catch (e) { failures++; console.log('  FAIL  ' + name + '\n        ' + (e.message || e)); }
}

// ---- WCAG relative luminance / contrast (mirrored for in-page asserts) ----
function srgb(c) { c /= 255; return c <= 0.03928 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4); }
function lum(hex) {
  const m = hex.replace('#', '');
  return 0.2126 * srgb(parseInt(m.slice(0, 2), 16)) +
         0.7152 * srgb(parseInt(m.slice(2, 4), 16)) +
         0.0722 * srgb(parseInt(m.slice(4, 6), 16));
}
/* Accept '#rrggbb' or computed 'rgb(r, g, b)' / 'rgba(r, g, b, a)'. */
function parseColor(str) {
  str = String(str).trim();
  if (str.charAt(0) === '#') return str.slice(0, 7);
  const m = str.match(/rgba?\(([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)/);
  if (!m) return str;
  const rgb = [m[1], m[2], m[3]].map(Number).map(v => Math.round(v).toString(16).padStart(2, '0'));
  return '#' + rgb.join('');
}
function contrast(a, b) { return (function (x, y) { const l1 = Math.max(lum(x), lum(y)), l2 = Math.min(lum(x), lum(y)); return (l1 + 0.05) / (l2 + 0.05); })(parseColor(a), parseColor(b)); }

async function main() {
  const chrome = spawn(CHROME, [
    '--headless=new', '--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage',
    '--remote-debugging-port=' + PORT, '--user-data-dir=' + fs.mkdtempSync(path.join('/tmp', 'ledgerly-e2e-')), 'about:blank'
  ]);
  chrome.stderr.on('data', () => {});

  let target = null;
  for (let i = 0; i < 60 && !target; i++) {
    await new Promise((r) => setTimeout(r, 250));
    for (const host of ['127.0.0.1', 'localhost']) {
      try { const list = await getJson(`http://${host}:${PORT}/json/list`); target = list.find((x) => x.type === 'page' && x.webSocketDebuggerUrl); if (target) break; } catch (e) {}
    }
  }
  if (!target) { console.error('no CDP target'); chrome.kill(); process.exit(1); }

  const ws = new WebSocket(target.webSocketDebuggerUrl);
  await new Promise((res, rej) => { ws.onopen = res; ws.onerror = rej; });

  let msgId = 0;
  const pending = new Map();
  const pageErrors = [];
  let loadCount = 0; // increments whenever a page finishes loading
  ws.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.id && pending.has(msg.id)) { pending.get(msg.id)(msg); pending.delete(msg.id); }
    if (msg.method === 'Page.loadEventFired') loadCount++;
    if (msg.method === 'Runtime.exceptionThrown') pageErrors.push(msg.params.exceptionDetails.exception ? msg.params.exceptionDetails.exception.description : msg.params.exceptionDetails.text);
    if (msg.method === 'Log.entryAdded' && msg.params.entry.level === 'error') pageErrors.push('console: ' + msg.params.entry.text);
  };
  /* Reload and wait for the NEW document's load event before continuing.
     Polling DOM conditions alone is racy: the previous document usually
     satisfies them too, so interactions could hit the dying page. */
  async function reloadAndWait() {
    const before = loadCount;
    await send('Page.reload', { ignoreCache: true });
    const deadline = Date.now() + 12000;
    while (Date.now() < deadline && loadCount <= before) await new Promise((r) => setTimeout(r, 100));
    if (loadCount <= before) throw new Error('timed out waiting for Page.loadEventFired');
    await new Promise((r) => setTimeout(r, 400)); // let deferred scripts finish
  }
  function send(method, params) { return new Promise((resolve) => { const id = ++msgId; pending.set(id, resolve); ws.send(JSON.stringify({ id, method, params })); }); }
  async function ev(expression) {
    const r = await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true });
    if (r.result && r.result.exceptionDetails) throw new Error('page exception: ' + (r.result.exceptionDetails.exception ? r.result.exceptionDetails.exception.description : JSON.stringify(r.result.exceptionDetails)));
    return r.result.result.value;
  }
  async function waitFor(expression, desc, timeoutMs) {
    const deadline = Date.now() + (timeoutMs || 12000);
    let last;
    while (Date.now() < deadline) {
      last = await ev(expression);
      if (last) return last;
      await new Promise((r) => setTimeout(r, 150));
    }
    throw new Error('timeout waiting for ' + desc + ' (last=' + JSON.stringify(last) + ')');
  }

  const ctx = { ev, waitFor, send, reloadAndWait };

  await send('Runtime.enable');
  await send('Log.enable');
  await send('Page.enable');
  await send('Page.navigate', { url: APP });
  await waitFor(`document.getElementById('search') !== null`, 'page shell');
  await ev(`localStorage.clear()`);
  await reloadAndWait();
  await waitFor(`document.getElementById('search') && document.querySelectorAll('.transaction-row').length === 10 && /Showing 10 of 10/.test(document.getElementById('results-meta').textContent)`, 'initial ready');
  await new Promise((r) => setTimeout(r, 800));

  /* =================================================================== *
   *  FUNCTIONAL — CRUD
   * ================================================================== */
  console.log('\n== CRUD & validation ==');
  await test('10 seed rows render with .transaction-row + .delete-transaction', async () => {
    const n = await ctx.ev(`document.querySelectorAll('.transaction-row').length`);
    assert.strictEqual(n, 10);
    const missing = await ctx.ev(`[...document.querySelectorAll('.transaction-row')].filter(r => !r.querySelector('.delete-transaction')).length`);
    assert.strictEqual(missing, 0);
  }, ctx);
  await test('summary cards match storage-derived totals', async () => {
    const s = await ctx.ev(`(function(){
      const txs = JSON.parse(localStorage.getItem('ledgerly-transactions'));
      const t = LedgerlyCore.computeTotals(txs);
      return [document.getElementById('summary-balance').textContent, LedgerlyUI.formatCurrency(t.balance),
              document.getElementById('summary-income').textContent, LedgerlyUI.formatCurrency(t.income),
              document.getElementById('summary-expense').textContent, LedgerlyUI.formatCurrency(t.expenses)];
    })()`);
    assert.deepStrictEqual([s[0], s[2], s[4]], [s[1], s[3], s[5]]);
  }, ctx);
  await test('empty description -> error in #form-error, focus moves to description, no row added', async () => {
    await ctx.ev(`(function(){
      document.getElementById('amount').value = '25';
      document.getElementById('date').value = '2025-01-10';
      document.getElementById('description').value = '   ';
      document.getElementById('transaction-form').dispatchEvent(new Event('submit', { cancelable: true }));
    })()`);
    const v = await ctx.ev(`({ err: document.getElementById('form-error').textContent, hidden: document.getElementById('form-error').hidden, active: document.activeElement.id, rows: document.querySelectorAll('.transaction-row').length })`);
    assert.ok(/description/i.test(v.err) && !v.hidden, JSON.stringify(v));
    assert.strictEqual(v.active, 'description');
    assert.strictEqual(v.rows, 10);
  }, ctx);
  await test('zero / negative / alphabetic amount -> error; focus moves to amount', async () => {
    for (const bad of ['0', '-5', 'abc', '0.004']) {
      await ctx.ev(`(function(){
        document.getElementById('description').value = 'Test';
        document.getElementById('amount').value = '${bad}';
        document.getElementById('date').value = '2025-01-10';
        document.getElementById('transaction-form').dispatchEvent(new Event('submit', { cancelable: true }));
      })()`);
      const v = await ctx.ev(`({ err: document.getElementById('form-error').textContent, hidden: document.getElementById('form-error').hidden, active: document.activeElement.id })`);
      assert.ok(/amount/i.test(v.err) && !v.hidden, 'amount=' + bad + ' ' + JSON.stringify(v));
      assert.strictEqual(v.active, 'amount');
    }
  }, ctx);
  await test('missing date -> error; focus moves to date', async () => {
    await ctx.ev(`(function(){
      document.getElementById('description').value = 'Test';
      document.getElementById('amount').value = '15';
      document.getElementById('date').value = '';
      document.getElementById('transaction-form').dispatchEvent(new Event('submit', { cancelable: true }));
    })()`);
    const v = await ctx.ev(`({ err: document.getElementById('form-error').textContent, active: document.activeElement.id })`);
    assert.ok(/date/i.test(v.err) && v.active === 'date', JSON.stringify(v));
  }, ctx);
  await test('valid add -> 11 rows, persisted, form cleared, summary updates', async () => {
    await ctx.ev(`(function(){
      document.getElementById('description').value = 'Client invoice';
      document.getElementById('amount').value = '320.50';
      const t = document.getElementById('type'); t.value = 'income'; t.dispatchEvent(new Event('change', { bubbles: true }));
      const c = document.getElementById('category'); c.value = 'Freelance'; c.dispatchEvent(new Event('change', { bubbles: true }));
      document.getElementById('date').value = '2025-02-14';
      document.getElementById('transaction-form').dispatchEvent(new Event('submit', { cancelable: true }));
    })()`);
    await ctx.waitFor(`document.querySelectorAll('.transaction-row').length === 11`, '11 rows');
    const v = await ctx.ev(`(function(){
      const stored = JSON.parse(localStorage.getItem('ledgerly-transactions'));
      const tx = stored.find(t => t.description === 'Client invoice');
      return { count: stored.length, tx: tx, errHidden: document.getElementById('form-error').hidden, desc: document.getElementById('description').value, date: document.getElementById('date').value !== '' };
    })()`);
    assert.strictEqual(v.count, 11);
    assert.ok(v.tx && v.tx.amount === 320.5 && v.tx.type === 'income' && v.tx.category === 'Freelance' && /^\d{4}-\d{2}-\d{2}$/.test(v.tx.date));
    assert.ok(v.errHidden && v.desc === '' && v.date);
  }, ctx);
  await test('delete row -> 10 rows, persisted, focus refocuses a .delete-transaction', async () => {
    await ctx.ev(`document.querySelector('.transaction-row .delete-transaction').dispatchEvent(new MouseEvent('click', { bubbles: true }))`);
    await ctx.waitFor(`document.querySelectorAll('.transaction-row').length === 10`, '10 rows after delete');
    const v = await ctx.ev(`({ stored: JSON.parse(localStorage.getItem('ledgerly-transactions')).length, active: document.activeElement.className })`);
    assert.strictEqual(v.stored, 10);
    assert.ok(/delete-transaction/.test(String(v.active)), 'focus=' + v.active);
  }, ctx);

  /* =================================================================== *
   *  FUNCTIONAL — FILTERS, SORT, SEARCH, RESET
   * ================================================================== */
  console.log('\n== filters, sort, search, reset ==');
  await test('search narrows; search + type + category compose', async () => {
    await ctx.ev(`(function(){ const s = document.getElementById('search'); s.value = 'salary'; s.dispatchEvent(new Event('input')); })()`);
    await ctx.waitFor(`document.querySelectorAll('.transaction-row').length === 1`, '1 row: salary');
    await ctx.ev(`(function(){ const t = document.getElementById('type-filter'); t.value = 'income'; t.dispatchEvent(new Event('change')); })()`);
    assert.ok(await ctx.waitFor(`document.querySelectorAll('.transaction-row').length === 1`, 'salary income'));
    await ctx.ev(`(function(){ const c = document.getElementById('category-filter'); c.value = 'Gifts'; c.dispatchEvent(new Event('change')); })()`);
    await ctx.waitFor(`document.querySelectorAll('.transaction-row').length === 0`, '0 rows: no match');
    const empty = await ctx.ev(`({ hidden: document.getElementById('empty-state').hidden, title: document.getElementById('empty-state-title').textContent, resetShown: !document.getElementById('empty-state-reset').hidden, tableHidden: document.getElementById('table-wrap').hidden })`);
    assert.deepStrictEqual({ h: empty.hidden, t: /No matching/.test(empty.title), r: empty.resetShown, tb: empty.tableHidden }, { h: false, t: true, r: true, tb: true });
  }, ctx);
  await test('reset-filters shows all rows again', async () => {
    await ctx.ev(`document.getElementById('reset-filters').click()`);
    await ctx.waitFor(`document.querySelectorAll('.transaction-row').length === 10`, '10 rows');
  }, ctx);
  await test('category filter + amount sort combine; asc reorders', async () => {
    await ctx.ev(`(function(){
      const c = document.getElementById('category-filter'); c.value = 'Transport'; c.dispatchEvent(new Event('change'));
      const s = document.getElementById('sort-by'); s.value = 'amount-desc'; s.dispatchEvent(new Event('change'));
    })()`);
    await ctx.waitFor(`document.querySelectorAll('.transaction-row').length === 2`, '2 Transport');
    assert.ok(/210/.test(await ctx.ev(`document.querySelector('.transaction-amount').textContent`)));
    await ctx.ev(`(function(){ const s = document.getElementById('sort-by'); s.value = 'amount-asc'; s.dispatchEvent(new Event('change')); })()`);
    const first = await ctx.waitFor(`document.querySelector('.transaction-amount').textContent`, 'first amount');
    assert.ok(first.includes('45') && !first.includes('210'), first);
  }, ctx);
  await ctx.ev(`document.getElementById('reset-filters').click()`);
  await ctx.waitFor(`document.querySelectorAll('.transaction-row').length === 10`, 'reset before budget');

  /* =================================================================== *
   *  FUNCTIONAL — BUDGET
   * ================================================================== */
  console.log('\n== budget ==');
  await test('budget save persists and progress shows current-month spend', async () => {
    await ctx.ev(`(function(){ document.getElementById('budget-input').value = '2000'; document.getElementById('save-budget').click(); })()`);
    await ctx.waitFor(`localStorage.getItem('ledgerly-budget') === '2000'`, 'budget stored');
    const spent = await ctx.ev(`LedgerlyCore.computeMonthlySpending(JSON.parse(localStorage.getItem('ledgerly-transactions')))`);
    const now = await ctx.ev(`document.getElementById('budget-progress').getAttribute('aria-valuenow')`);
    assert.strictEqual(parseInt(now, 10), Math.min(Math.round(spent / 2000 * 100), 100));
  }, ctx);
  await test('overspend: real % in text, bar clamped, no NaN', async () => {
    await ctx.ev(`(function(){ document.getElementById('budget-input').value = '1'; document.getElementById('save-budget').click(); })()`);
    await ctx.waitFor(`/Overspent/.test(document.getElementById('budget-status').textContent)`, 'overspent');
    const v = await ctx.ev(`({ now: document.getElementById('budget-progress').getAttribute('aria-valuenow'), width: document.getElementById('budget-bar').style.width, status: document.getElementById('budget-status').textContent })`);
    assert.strictEqual(v.now, '100');
    assert.strictEqual(v.width, '100%');
    assert.ok(!/NaN|Infinity/.test(v.status));
  }, ctx);
  await test('zero budget: paused state, no invalid percent, persisted 0', async () => {
    await ctx.ev(`(function(){ document.getElementById('budget-input').value = '0'; document.getElementById('save-budget').click(); })()`);
    await ctx.waitFor(`/paused/.test(document.getElementById('budget-status').textContent)`, 'paused');
    const v = await ctx.ev(`({ now: document.getElementById('budget-progress').getAttribute('aria-valuenow'), width: document.getElementById('budget-bar').style.width, status: document.getElementById('budget-status').textContent, stored: localStorage.getItem('ledgerly-budget') })`);
    assert.strictEqual(v.now, '0');
    assert.strictEqual(v.width, '0%');
    assert.strictEqual(v.stored, '0');
    assert.ok(!/NaN|Infinity/.test(v.status));
  }, ctx);
  await test('invalid budget values give inline feedback and do not persist', async () => {
    for (const bad of ['-5', 'abc', '']) {
      await ctx.ev(`(function(){ document.getElementById('budget-input').value = '${bad}'; document.getElementById('save-budget').click(); })()`);
      const v = await ctx.ev(`({ isErr: document.getElementById('budget-message').classList.contains('is-error'), stored: localStorage.getItem('ledgerly-budget') })`);
      assert.ok(v.isErr, 'budget ' + JSON.stringify(bad) + ' should error');
      assert.strictEqual(v.stored, '0');
    }
  }, ctx);
  await ctx.ev(`(function(){ document.getElementById('budget-input').value = '2000'; document.getElementById('save-budget').click(); })()`);

  /* =================================================================== *
   *  FUNCTIONAL — THEME
   * ================================================================== */
  console.log('\n== theme ==');
  await test('theme toggle flips html[data-theme] to opposite, persists raw string, restores on reload', async () => {
    const before = await ctx.ev(`({ theme: document.documentElement.getAttribute('data-theme'), stored: localStorage.getItem('ledgerly-theme') })`);
    const opposite = before.theme === 'dark' ? 'light' : 'dark';
    await ctx.ev(`document.getElementById('theme-toggle').click()`);
    let v = await ctx.ev(`({ theme: document.documentElement.getAttribute('data-theme'), stored: localStorage.getItem('ledgerly-theme'), pressed: document.getElementById('theme-toggle').getAttribute('aria-pressed') })`);
    assert.strictEqual(v.theme, opposite);
    assert.strictEqual(v.stored, opposite);
    assert.strictEqual(v.pressed, String(opposite === 'dark'));
    // now verify persistence across reload
    await ctx.reloadAndWait();
    await ctx.waitFor(`document.getElementById('search') && document.querySelectorAll('.transaction-row').length === 10`, 'reload ready');
    const after = await ctx.ev(`document.documentElement.getAttribute('data-theme')`);
    assert.strictEqual(after, opposite);
    // restore light-ish state for later tests
    if (after !== before.theme) { await ctx.ev(`document.getElementById('theme-toggle').click()`); }
  }, ctx);

  /* =================================================================== *
   *  ACCESSIBILITY — static structure
   * ================================================================== */
  console.log('\n== accessibility: structure & labels ==');
  await test('exactly one h1, no skipped heading levels', async () => {
    const h = await ctx.ev(`[...document.querySelectorAll('h1,h2,h3,h4,h5,h6')].map(el => Number(el.tagName[1]))`);
    assert.strictEqual(h[0], 1, 'first heading must be h1, got ' + h[0]);
    assert.ok(h.filter(l => l === 1).length === 1, 'exactly one h1');
    for (let i = 1; i < h.length; i++) assert.ok(h[i] <= h[i - 1] + 1, 'skip in levels: ' + h.join(','));
  }, ctx);
  await test('no duplicate element ids', async () => {
    const dup = await ctx.ev(`(function(){ const seen={}, bad=[]; document.querySelectorAll('[id]').forEach(el => { if (seen[el.id]) bad.push(el.id); seen[el.id]=1; }); return bad; })()`);
    assert.deepStrictEqual(dup, []);
  }, ctx);
  await test('every form control in the form has an associated label', async () => {
    const bad = await ctx.ev(`(function(){
      const out = [];
      document.querySelectorAll('#transaction-form input, #transaction-form select').forEach(el => {
        const ok = el.labels && el.labels.length > 0 || el.getAttribute('aria-label') || el.getAttribute('aria-labelledby');
        if (!ok) out.push(el.id || el.name || el.tagName);
      });
      return out;
    })()`);
    assert.deepStrictEqual(bad, []);
  }, ctx);
  await test('form controls point at #form-error via aria-describedby', async () => {
    const bad = await ctx.ev(`[...document.querySelectorAll('#transaction-form input, #transaction-form select')].filter(el => (el.getAttribute('aria-describedby') || '').split(/\\s+/).includes('form-error')).length`);
    assert.strictEqual(bad, 5);
  }, ctx);
  await test('all buttons have accessible names', async () => {
    const bad = await ctx.ev(`[...document.querySelectorAll('button')].filter(b => {
      const name = (b.textContent || '').trim() || b.getAttribute('aria-label') || b.getAttribute('title');
      return !name;
    }).map(b => b.id || b.className)`);
    assert.deepStrictEqual(bad, []);
  }, ctx);
  await test('filters toolbar uses a group role (not a roving-tab toolbar)', async () => {
    const role = await ctx.ev(`document.querySelector('.filters').getAttribute('role')`);
    assert.strictEqual(role, 'group');
  }, ctx);
  await test('skip link is first focusable element with target #main-content', async () => {
    const v = await ctx.ev(`(function(){ const s = document.querySelector('.skip-link'); return { href: s && s.getAttribute('href'), first: s === document.querySelector('a,button,input,select') }; })()`);
    assert.strictEqual(v.href, '#main-content');
    assert.strictEqual(v.first, true);
  }, ctx);
  await test('progressbar has valid aria-valuemin/max/now (clamped 0..100)', async () => {
    const v = await ctx.ev(`(function(){ const p = document.getElementById('budget-progress'); return { min: Number(p.getAttribute('aria-valuemin')), max: Number(p.getAttribute('aria-valuemax')), now: Number(p.getAttribute('aria-valuenow')) }; })()`);
    assert.strictEqual(v.min, 0);
    assert.strictEqual(v.max, 100);
    assert.ok(Number.isFinite(v.now) && v.now >= 0 && v.now <= 100, JSON.stringify(v));
  }, ctx);

  /* =================================================================== *
   *  ACCESSIBILITY — keyboard + contrast
   * ================================================================== */
  console.log('\n== accessibility: keyboard & contrast ==');
  await test('Tab reaches theme toggle; element actively matches :focus-visible; outline is solid 3px', async () => {
    for (let i = 0; i < 3; i++) {
      await ctx.send('Input.dispatchKeyEvent', { type: 'keyDown', key: 'Tab', code: 'Tab', windowsVirtualKeyCode: 9 });
      await ctx.send('Input.dispatchKeyEvent', { type: 'keyUp', key: 'Tab', code: 'Tab', windowsVirtualKeyCode: 9 });
      await new Promise((r) => setTimeout(r, 120));
    }
    const v = await ctx.ev(`(function(){ const el = document.activeElement; return { id: el.id, matches: el.matches(':focus-visible'), outline: getComputedStyle(el).outlineStyle + ' ' + getComputedStyle(el).outlineWidth, colorsMatch: getComputedStyle(document.body).backgroundColor !== getComputedStyle(document.body).color }; })()`);
    assert.strictEqual(v.id, 'theme-toggle');
    assert.strictEqual(v.matches, true);
    assert.strictEqual(v.outline, 'solid 3px');
  }, ctx);
  await test('WCAG AA 4.5:1 contrast in light theme', async () => {
    await ctx.ev(`localStorage.setItem('ledgerly-theme','light'); document.documentElement.setAttribute('data-theme','light');`);
    await new Promise((r) => setTimeout(r, 300));
    const results = await ctx.ev(`(function(){
      const cs = getComputedStyle(document.body);
      const pick = sel => getComputedStyle(document.querySelector(sel)).color;
      const pairs = [
        ['body text', '131a2b', 'ffffff'],
        ['muted text', '5d6779', 'ffffff'],
        ['income amount', pick('.amount--income'), 'ffffff'],
        ['expense amount', pick('.amount--expense'), 'ffffff'],
        ['accent on surface', '5148e8', 'ffffff'],
        ['primary btn white text', 'ffffff', '5148e8']
      ];
      return pairs;
    })()`);
    for (const [label, fg, bg] of results) {
      const r = contrast(fg, bg);
      assert.ok(r >= 4.5, label + ' ' + fg + ' on ' + bg + ' = ' + r.toFixed(2));
    }
  }, ctx);
  await test('WCAG AA 4.5:1 contrast in dark theme', async () => {
    await ctx.ev(`localStorage.setItem('ledgerly-theme','dark'); document.documentElement.setAttribute('data-theme','dark');`);
    await new Promise((r) => setTimeout(r, 300));
    const results = await ctx.ev(`(function(){
      const cs = getComputedStyle(document.body);
      const surface = '171b2e';
      const pick = sel => getComputedStyle(document.querySelector(sel)).color;
      return [
        ['body text', 'eef1fb', surface],
        ['muted text', '98a2bd', surface],
        ['income amount', pick('.amount--income'), surface],
        ['expense amount', pick('.amount--expense'), surface],
        ['accent text', '818cf8', surface],
        ['accent-solid btn white text', 'ffffff', '5659e8']
      ];
    })()`);
    for (const [label, fg, bg] of results) {
      const r = contrast(fg, bg);
      assert.ok(r >= 4.5, label + ' ' + fg + ' on ' + bg + ' = ' + r.toFixed(2));
    }
  }, ctx);

  /* =================================================================== *
   *  RESPONSIVE LAYOUT
   * ================================================================== */
  console.log('\n== responsive layout ==');
  const sizes = [320, 390, 768, 1024];
  for (const w of sizes) {
    await test('no horizontal page overflow at ' + w + 'px', async () => {
      await ctx.send('Emulation.setDeviceMetricsOverride', { width: w, height: 900, deviceScaleFactor: 1, mobile: w <= 768 });
      await new Promise((r) => setTimeout(r, 250));
      const v = await ctx.ev(`({ sw: document.documentElement.scrollWidth, cw: document.documentElement.clientWidth })`);
      assert.ok(v.sw <= v.cw + 1, JSON.stringify(v));
    }, ctx);
  }
  await test('mobile <=600px: table becomes stacked cards (flex rows, hidden thead)', async () => {
    await ctx.send('Emulation.setDeviceMetricsOverride', { width: 390, height: 844, deviceScaleFactor: 1, mobile: true });
    await new Promise((r) => setTimeout(r, 300));
    const v = await ctx.ev(`({ rowDisp: getComputedStyle(document.querySelector('.transaction-row')).display, thead: getComputedStyle(document.querySelector('.transactions-table thead')).position, wrapperOverflow: getComputedStyle(document.querySelector('.table-wrap')).overflowX, summary: getComputedStyle(document.querySelector('.summary')).gridTemplateColumns })`);
    assert.strictEqual(v.rowDisp, 'flex');
    assert.strictEqual(v.thead, 'absolute');
    assert.ok(v.summary.split(' ').length === 1, 'summary should be single column, got ' + v.summary);
  }, ctx);
  await test('desktop >=980px: 3-column summary + 2-column dashboard grid', async () => {
    await ctx.send('Emulation.setDeviceMetricsOverride', { width: 1440, height: 1100, deviceScaleFactor: 1, mobile: false });
    await new Promise((r) => setTimeout(r, 300));
    const v = await ctx.ev(`({ summary: getComputedStyle(document.querySelector('.summary')).gridTemplateColumns, dash: getComputedStyle(document.querySelector('.dashboard')).gridTemplateColumns })`);
    assert.strictEqual(v.summary.split(' ').length, 3);
    assert.strictEqual(v.dash.split(' ').length, 2);
  }, ctx);

  /* =================================================================== *
   *  PERSISTENCE — restart / corrupt-data hygiene
   * ================================================================== */
  console.log('\n== persistence ==');
  await test('full page reload restores all data from localStorage', async () => {
    await ctx.send('Emulation.clearDeviceMetricsOverride');
    await ctx.reloadAndWait();
    await ctx.waitFor(`document.getElementById('search') && document.querySelectorAll('.transaction-row').length === 10`, 'reload ready');
    const v = await ctx.ev(`({ rows: document.querySelectorAll('.transaction-row').length, budget: localStorage.getItem('ledgerly-budget'), theme: document.documentElement.getAttribute('data-theme') })`);
    assert.strictEqual(v.rows, 10);
    assert.strictEqual(v.budget, '2000');
    assert.strictEqual(v.theme, 'dark'); // last saved
  }, ctx);
  await test('garbage entries in stored data are sanitized on load', async () => {
    await ctx.ev(`localStorage.setItem('ledgerly-transactions', JSON.stringify([
      { id: 'hack', description: '<img src=x>', amount: -5, type: 'expense', category: 'X', date: 'bad' },
      { id: 'dup', description: 'First', amount: 10, type: 'expense', category: 'Housing', date: '2025-01-01', createdAt: 1 },
      { id: 'dup', description: 'Second', amount: 99, type: 'income', category: 'Salary', date: '2025-01-02', createdAt: 2 },
      { description: 'No id', amount: 5, type: 'income', category: 'Freelance', date: '2025-01-03' }
    ]))`);
    await ctx.reloadAndWait();
    // 4 stored entries: 1 malformed (dropped), 1 duplicate id (dropped),
    // 2 valid => exactly 2 rows: 'First' (dup id kept once) and 'No id'.
    await ctx.waitFor(`document.getElementById('search') && document.querySelectorAll('.transaction-row').length === 2`, '2 sanitized rows');
    const v = await ctx.ev(`({ stored: JSON.parse(localStorage.getItem('ledgerly-transactions')).map(t => t.description), balance: document.getElementById('summary-balance').textContent })`);
    assert.deepStrictEqual(v.stored, ['First', 'No id']);
    const expected = await ctx.ev(`LedgerlyUI.formatCurrency(LedgerlyCore.computeTotals(JSON.parse(localStorage.getItem('ledgerly-transactions'))).balance)`);
    assert.strictEqual(v.balance, expected);
  }, ctx);
  await test('delete-all -> "No transactions yet"; reload does NOT reseed', async () => {
    for (let i = 0; i < 20; i++) {
      const n = await ctx.ev(`document.querySelectorAll('.transaction-row').length`);
      if (n === 0) break;
      await ctx.ev(`document.querySelector('.delete-transaction').dispatchEvent(new MouseEvent('click', { bubbles: true }))`);
      await ctx.waitFor(`document.querySelectorAll('.transaction-row').length === ${n - 1}`, 'delete to ' + (n - 1));
    }
    await ctx.waitFor(`document.getElementById('summary-balance').textContent === '$0.00'`, 'all deleted');
    const v = await ctx.ev(`({ stored: JSON.stringify(JSON.parse(localStorage.getItem('ledgerly-transactions'))), title: document.getElementById('empty-state-title').textContent, hidden: document.getElementById('empty-state').hidden })`);
    assert.strictEqual(v.stored, '[]');
    assert.ok(/No transactions yet/.test(v.title));
    assert.strictEqual(v.hidden, false);
    await ctx.reloadAndWait();
    await ctx.waitFor(`document.getElementById('empty-state-title') && document.getElementById('empty-state-title').textContent === 'No transactions yet'`, 'still empty');
    assert.strictEqual(await ctx.ev(`document.querySelectorAll('.transaction-row').length`), 0);
  }, ctx);
  await test('adding after empty state works; single-transaction grammar correct', async () => {
    await ctx.ev(`(function(){
      document.getElementById('description').value = 'Bus pass';
      document.getElementById('amount').value = '30';
      document.getElementById('date').value = '2025-06-01';
      document.getElementById('transaction-form').dispatchEvent(new Event('submit', { cancelable: true }));
    })()`);
    await ctx.waitFor(`document.querySelectorAll('.transaction-row').length === 1`, 'one row');
    const v = await ctx.ev(`({ meta: document.getElementById('results-meta').textContent, tableHidden: document.getElementById('table-wrap').hidden, emptyHidden: document.getElementById('empty-state').hidden })`);
    assert.strictEqual(v.meta, 'Showing 1 of 1 transaction');
    assert.strictEqual(v.tableHidden, false);
    assert.strictEqual(v.emptyHidden, true);
  }, ctx);

  /* =================================================================== *
   *  NO UNCAUGHT PAGE ERRORS
   * ================================================================== */
  console.log('\n== page errors ==');
  await test('no uncaught page exceptions or console errors during the whole run', async () => {
    if (pageErrors.length) throw new Error(pageErrors.join(' | '));
  }, ctx);

  ws.close();
  chrome.kill();
  console.log(failures ? '\n' + failures + ' E2E FAILURE(S)' : '\nAll E2E checks passed');
  process.exit(failures ? 1 : 0);
}

main().catch((e) => { console.error('driver error:', e); process.exit(1); });
