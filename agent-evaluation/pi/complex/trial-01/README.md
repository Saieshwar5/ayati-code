# Ledgerly

A polished, responsive personal-finance dashboard built with **only HTML, CSS, and vanilla JavaScript** — no frameworks, no build step, no CDNs, and no network resources.

Record income and expenses, explore them with combinable search/filters/sorting, and watch a monthly budget progress bar that safely handles zero budgets and overspending.

![tech](https://img.shields.io/badge/stack-HTML%20%2B%20CSS%20%2B%20Vanilla%20JS-6366f1?style=flat)

---

## Running the site

Ledgerly is a static site. Any basic HTTP server works; open the directory and serve it:

```bash
# Option 1 — Python
python3 -m http.server 8000

# Option 2 — Node
npx serve .
```

Then visit <http://localhost:8000>.

> **Why a server?** The page reads/writes `localStorage`, which works fine from `file://` in most browsers, but a static server is the supported way to run it and keeps everything deterministic.

Because there are no external resources, the site works fully offline once the files are on your machine.

## Feature checklist

- **Summary cards** — current balance, total income, and total expenses, always kept in sync with the ledger.
- **Transaction table** — every row is `<tr class="transaction-row">` with a `.delete-transaction` button; on phones the table reflows into stacked cards.
- **Add / delete transactions** — the form (`#transaction-form`) validates descriptions, amounts (> 0, numeric), and dates, showing feedback in `#form-error` without reloading the page.
- **Composable exploration** — text search (`#search`), type filter (`#type-filter`), category filter (`#category-filter`), and sort (`#sort-by`) all work together, plus a one-click `#reset-filters` control and helpful empty states for "nothing recorded" vs. "nothing matches".
- **Monthly budget** — `#budget-input` + `#save-budget` persist a budget; a visible progress indicator (`#budget-progress`) tracks the current month's expenses. Zero budgets show "tracking paused", overspending shows the true percentage in text while the bar clamps to 100% so no layout ever breaks.
- **Theming** — a light/dark toggle (`#theme-toggle`) persisted under `ledgerly-theme`; first-time visitors inherit their OS preference.
- **Accessibility** — semantic landmarks, labeled controls, a skip link, visible `:focus-visible` rings, keyboard-operable filters/buttons, `aria-live` announcements, and `prefers-reduced-motion` support.

## Seeding

On the very first visit (no `ledgerly-transactions` key yet), Ledgerly writes **10 realistic seed transactions**, dated within the current and previous month so the budget tracker is meaningful immediately. Seeds are persisted once and never re-inserted — if you delete every transaction, the app happily shows the "No transactions yet" state instead of resurrecting the demo data.

## Architecture

The JavaScript is deliberately split by responsibility (no modules, no bundler — plain script tags in dependency order):

```
index.html
├── css/
│   └── styles.css          Theming tokens, layout, table-card reflow, motion
├── js/
│   ├── core.js             # Pure data + logic (validation, totals, budget math,
│   │                       #   filtering/sorting, category taxonomy, seed data,
│   │                       #   stored-data sanitization)
│   ├── storage.js          # Defensive localStorage wrapper + seeding policy
│   ├── ui.js               # View layer: builds the DOM, formats currency/dates
│   └── app.js              # Controller: state, event wiring, orchestration
└── tests/
    ├── logic.test.js       # Node-only logic + storage tests (no browser)
    └── e2e.js              # Headless-Chromium interaction & audit suite
```

```
┌────────────┐   state + events    ┌─────────────┐
│  app.js    │ ──────────────────► │  core.js    │  pure, testable logic
│ (controller)│  ◄───────────────── │  (logic)    │  no DOM, no storage
└─────┬──────┘         results     └─────────────┘
      │  persist / hydrate
      ▼
┌────────────┐          ┌─────────────┐
│ storage.js │          │  ui.js      │  renders rows, cards, progress
└────────────┘          └─────────────┘
   localStorage             DOM
```

**Why this split?** `core.js` never touches `document`, `window.localStorage`, or events, so it can be unit-tested in Node with zero mocks — the same functions that power the dashboard drive the test suite.

## Storage

Everything lives in `localStorage` under three namespaced keys:

| Key                      | Format          | Contents                              |
| ------------------------ | --------------- | ------------------------------------- |
| `ledgerly-transactions`  | JSON array      | `{id, description, amount, type, category, date, createdAt}` |
| `ledgerly-budget`        | JSON number     | Monthly budget; `null`-free — `0` = explicitly "no budget" |
| `ledgerly-theme`         | plain string    | `"light"` or `"dark"` (readable by an inline head script before first paint, preventing a theme flash) |

`storage.js` wraps every access in `try/catch`, so blocked/private storage degrades gracefully (in-memory operation, console warning) instead of crashing the app. Amounts are rounded to whole cents; dates are stored as `YYYY-MM-DD` and months are derived from the first 7 characters (`YYYY-MM`), which makes the budget tracker and date sorting trivial and timezone-safe.

## Verification

Two self-contained test suites live in `tests/`:

```bash
# 1) Node logic suite — no browser needed (stubs localStorage)
node tests/logic.test.js

# 2) End-to-end browser audit — real headless Chromium over CDP (Node 22+)
python3 -m http.server 8765 &          # any static server works
node tests/e2e.js                      # or: CHROME_BIN=/path/to/chromium
```

The E2E suite drives the actual UI: CRUD + validation, composable
filters/sort/search/reset, budget save/overspend/zero/invalid states, theme
toggle + reload persistence, delete-all empty state, sanitized reload of
tampered `localStorage`, plus accessibility (single `h1`, no skipped heading
levels, unique ids, labeled controls, `aria-describedby`, accessible button
names, `:focus-visible` outline, WCAG AA 4.5:1 contrast in both themes) and
responsive-layout checks (no horizontal page overflow from 320 px through
1440 px, stacked-card table on phones, two-column dashboard on desktop).

`core.js` is pure (no DOM/localStorage) so the same code that powers the
dashboard drives the Node suite.

### Audit history (notable defects found & fixed)

- **Dark-theme income/expense colors**: the dark palette never overrode
  `--income`/`--expense`, so dark mode reused the light-theme green/red
  (3.1:1 contrast — failing AA). Dark mode now uses bright emerald/red
  (`#34d399`/`#f87171`) and both themes keep AA-safe `--accent-solid` for
  white-on-solid buttons.
- **`localStorage` hygiene**: stored transactions are now sanitized on load
  (malformed/duplicate entries dropped) and the cleaned list is persisted
  back, so tampered/corrupt data can neither crash rendering nor resurface.
- **Sub-cent amounts**: values below $0.005 that rounded to $0.00 (e.g.
  `0.004`) passed validation but added a zero-value row. Validation now
  requires the *rounded* amount to be at least $0.01.
- **Horizontal page overflow at ~1024 px**: the wide table's scroll overflow
  leaked into the document via the uncontained absolutely-positioned
  `sr-only` caption; `.table-wrap` is now `position: relative` (and the grid
  panel is `min-width: 0`), so the table scrolls inside its wrapper instead.
- **A11y structure**: added a page-level `h1` (brand name), switched the
  filter bar from `role="toolbar"` (which implies roving-tab keyboard
  navigation that was not implemented) to `role="group"`, wired all five
  form controls to `#form-error` via `aria-describedby`, and made
  error-focus routing precise (focus lands on the specific invalid control).
