# Ledgerly

A polished, responsive personal-finance dashboard built with **only HTML, CSS, and vanilla JavaScript** — no frameworks, no external packages, no CDNs, and no network resources.

## Features

- **Summary cards** for current balance, total income, and total expenses.
- **Transaction table** with add and delete support.
- **Filters that work together**: text search, type filter, category filter, and sorting.
- **Monthly budget** with a progress bar, zero-budget handling, and overspending detection.
- **Light / dark theme** toggle.
- **Fully responsive** mobile and desktop layouts.
- **Accessible**: semantic HTML, keyboard navigation, visible focus styles, ARIA labels, and `prefers-reduced-motion` support.
- **Empty states** for both "no data" and "no filter matches".

## How to run

The site is static and needs only a basic HTTP server (it uses `localStorage`, which requires a real origin rather than `file://` in most browsers).

From the project root:

```bash
# Python 3
python3 -m http.server 8000

# or Node
npx serve .
```

Then open <http://localhost:8000> in your browser.

## Architecture

```
index.html          Entry point — semantic page structure and all controls.
css/styles.css      All styling, theming via CSS custom properties, responsive breakpoints.
js/storage.js       Storage layer — safe localStorage read/write wrappers.
js/theme.js         Theme controller — applies and persists light/dark mode.
js/transactions.js  Transaction model, seed data, formatting, and row rendering.
js/app.js           Application controller — form, filters, sorting, budget, rendering.
```

Scripts load in dependency order (`storage.js` → `theme.js` → `transactions.js` → `app.js`). Each file is an IIFE exposing a small global namespace (`Storage`, `Theme`, `Transactions`, `App`), keeping concerns separated without any build step.

## Storage

All data persists in the browser's `localStorage` under these keys:

| Key                     | Contents                                        |
| ----------------------- | ----------------------------------------------- |
| `ledgerly-transactions` | JSON array of transaction objects               |
| `ledgerly-budget`       | Monthly budget number (or `null` when unset)    |
| `ledgerly-theme`        | `"light"` or `"dark"`                           |

A transaction object looks like:

```json
{
  "id": "unique-id",
  "description": "Weekly groceries",
  "amount": 86.4,
  "type": "expense",
  "category": "Food & Dining",
  "date": "2025-08-05"
}
```

On first load, when no saved transactions exist, Ledgerly seeds **seven realistic transactions** so the dashboard is immediately useful. Once you add or delete a transaction, your own data is saved and the seed data is never re-inserted.

## Notes

- Amounts are validated to be finite and greater than zero; descriptions and dates are required.
- The budget progress bar clamps to 100% and switches to a warning/over state, so zero budgets and overspending never produce invalid percentages or broken layouts.
- Clearing the budget input and pressing **Save Budget** removes the budget.
