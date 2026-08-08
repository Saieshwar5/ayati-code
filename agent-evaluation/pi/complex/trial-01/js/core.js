/*!
 * Ledgerly — core module.
 * Pure data + logic helpers with no DOM dependency, so the same code can be
 * unit-tested in Node and run in the browser.
 */
(function (global) {
  'use strict';

  /* ------------------------------------------------------------------ *
   * Taxonomy
   * ------------------------------------------------------------------ */

  var CATEGORIES = {
    income: ['Freelance', 'Gifts', 'Investments', 'Refunds', 'Salary'],
    expense: [
      'Dining Out', 'Entertainment', 'Groceries', 'Health', 'Housing',
      'Shopping', 'Subscriptions', 'Transport', 'Utilities'
    ]
  };

  var ALL_CATEGORIES = (function () {
    var set = {};
    Object.keys(CATEGORIES).forEach(function (kind) {
      CATEGORIES[kind].forEach(function (cat) { set[cat] = true; });
    });
    return Object.keys(set).sort();
  })();

  /* ------------------------------------------------------------------ *
   * Dates & money helpers
   * ------------------------------------------------------------------ */

  function pad2(n) { return (n < 10 ? '0' : '') + n; }

  function toISODate(d) {
    return d.getFullYear() + '-' + pad2(d.getMonth() + 1) + '-' + pad2(d.getDate());
  }

  function currentISODate() { return toISODate(new Date()); }

  function currentMonthKey() { return currentISODate().slice(0, 7); }

  function roundMoney(n) { return Math.round(n * 100) / 100; }

  function isValidISODate(value) {
    if (typeof value !== 'string' || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return false;
    var parts = value.split('-').map(Number);
    var d = new Date(parts[0], parts[1] - 1, parts[2]);
    return d.getFullYear() === parts[0] && d.getMonth() === parts[1] - 1 && d.getDate() === parts[2];
  }

  /* ------------------------------------------------------------------ *
   * Seed data — inserted only when no saved transactions exist.
   * ------------------------------------------------------------------ */

  /* A day in the *current* month, offset by `days` (never before the 1st),
     so the seeded data always exercises the monthly budget tracker. */
  function currentMonthDate(days) {
    var now = new Date();
    var day = Math.max(1, now.getDate() - days);
    return toISODate(new Date(now.getFullYear(), now.getMonth(), day));
  }

  /* A day in the previous month, for an all-time-looking transaction list. */
  function previousMonthDate(day) {
    var now = new Date();
    return toISODate(new Date(now.getFullYear(), now.getMonth() - 1, day));
  }

  function createSeedTransactions() {
    return [
      { id: 'seed-01', description: 'Monthly salary',    amount: 5200,  type: 'income',  category: 'Salary',        date: currentMonthDate(1),  createdAt: 1 },
      { id: 'seed-02', description: 'Rent payment',      amount: 1450,  type: 'expense', category: 'Housing',       date: currentMonthDate(0),  createdAt: 2 },
      { id: 'seed-03', description: 'Grocery run',       amount: 86.42, type: 'expense', category: 'Groceries',     date: currentMonthDate(2),  createdAt: 3 },
      { id: 'seed-04', description: 'Freelance invoice', amount: 640,   type: 'income',  category: 'Freelance',     date: currentMonthDate(4),  createdAt: 4 },
      { id: 'seed-05', description: 'Dinner with friends', amount: 38.75, type: 'expense', category: 'Dining Out',  date: currentMonthDate(5),  createdAt: 5 },
      { id: 'seed-06', description: 'Electric bill',     amount: 118.55, type: 'expense', category: 'Utilities',    date: currentMonthDate(3),  createdAt: 6 },
      { id: 'seed-07', description: 'Streaming subscription', amount: 12.99, type: 'expense', category: 'Subscriptions', date: currentMonthDate(6), createdAt: 7 },
      { id: 'seed-08', description: 'Gas station fill-up', amount: 45,  type: 'expense', category: 'Transport',     date: currentMonthDate(7),  createdAt: 8 },
      { id: 'seed-09', description: 'Car insurance',     amount: 210,   type: 'expense', category: 'Transport',     date: previousMonthDate(12), createdAt: 9 },
      { id: 'seed-10', description: 'Birthday gift from grandma', amount: 100, type: 'income', category: 'Gifts', date: previousMonthDate(9), createdAt: 10 }
    ];
  }

  function makeId() {
    return 'tx-' + Date.now().toString(36) + '-' + Math.random().toString(36).slice(2, 8);
  }

  /* ------------------------------------------------------------------ *
   * Validation
   * ------------------------------------------------------------------ */

  /**
   * @param {Object} input  { description, amount, type, category, date }
   * @returns {{valid:boolean, errors:string[], transaction:Object|null}}
   */
  function validateTransaction(input) {
    var errors = [];

    var description = String(input.description == null ? '' : input.description).trim();
    if (!description) errors.push('Please enter a description.');

    var rawAmount = String(input.amount == null ? '' : input.amount).trim();
    var amount = Number(rawAmount);
    // Rounding can silently collapse tiny positive amounts to $0.00, so the
    // *rounded* value must still be positive (at least half a cent).
    var roundedAmount = isFinite(amount) ? roundMoney(amount) : NaN;
    if (rawAmount === '' || !isFinite(amount) || amount <= 0 || roundedAmount <= 0) {
      errors.push('Please enter an amount of at least $0.01.');
    }

    var date = String(input.date == null ? '' : input.date).trim();
    if (!date) errors.push('Please choose a date.');
    else if (!isValidISODate(date)) errors.push('Please choose a valid date.');

    var type = input.type === 'income' ? 'income' : 'expense';

    var category = String(input.category == null ? '' : input.category).trim();
    if (!category) errors.push('Please choose a category.');

    if (errors.length) return { valid: false, errors: errors, transaction: null };

    return {
      valid: true,
      errors: [],
      transaction: {
        description: description,
        amount: roundedAmount,
        type: type,
        category: category,
        date: date
      }
    };
  }

  /* ------------------------------------------------------------------ *
   * Stored-data hygiene
   * ------------------------------------------------------------------ */

  /**
   * Normalize an array of stored transactions, dropping entries that are
   * malformed, non-finite, or duplicate. Guards rendering (and totals)
   * against tampered or partially corrupted localStorage content.
   */
  function sanitizeTransactions(list) {
    if (!Array.isArray(list)) return [];
    var seen = {};
    var out = [];
    list.forEach(function (item) {
      if (!item || typeof item !== 'object') return;
      var description = String(item.description == null ? '' : item.description).trim();
      var amount = Number(item.amount);
      var category = String(item.category == null ? '' : item.category).trim();
      var type = item.type === 'income' ? 'income' : 'expense';
      var date = String(item.date == null ? '' : item.date).trim();
      var id = typeof item.id === 'string' && item.id.trim() ? item.id : null;
      if (!description || !isFinite(amount) || amount <= 0 || roundMoney(amount) <= 0 ||
          !isValidISODate(date) || !category) {
        return;
      }
      if (id) {
        if (seen[id]) return;
        seen[id] = true;
      }
      out.push({
        id: id || makeId(),
        description: description.slice(0, 120),
        amount: roundMoney(amount),
        type: type,
        category: category,
        date: date,
        createdAt: typeof item.createdAt === 'number' && isFinite(item.createdAt) ? item.createdAt : Date.now()
      });
    });
    return out;
  }

  /* ------------------------------------------------------------------ *
   * Totals & budget
   * ------------------------------------------------------------------ */

  function computeTotals(transactions) {
    var income = 0;
    var expenses = 0;
    var incomeCount = 0;
    var expenseCount = 0;
    (transactions || []).forEach(function (tx) {
      if (tx.type === 'income') { income += tx.amount; incomeCount += 1; }
      else { expenses += tx.amount; expenseCount += 1; }
    });
    income = roundMoney(income);
    expenses = roundMoney(expenses);
    return {
      income: income,
      expenses: expenses,
      balance: roundMoney(income - expenses),
      incomeCount: incomeCount,
      expenseCount: expenseCount
    };
  }

  function computeMonthlySpending(transactions) {
    var key = currentMonthKey();
    var total = 0;
    (transactions || []).forEach(function (tx) {
      if (tx.type === 'expense' && String(tx.date).slice(0, 7) === key) total += tx.amount;
    });
    return roundMoney(total);
  }

  /**
   * Budget statistics. Never returns Infinity/NaN percentages:
   * a missing or zero budget is treated as "tracking paused".
   */
  function budgetStats(transactions, budget) {
    var spent = computeMonthlySpending(transactions);
    var percent = 0;
    if (budget != null && budget > 0) {
      percent = roundMoney((spent / budget) * 100);
    }
    return {
      spent: spent,
      percent: percent,
      over: budget > 0 ? Math.max(0, roundMoney(spent - budget)) : 0
    };
  }

  /* ------------------------------------------------------------------ *
   * Filtering & sorting — all filters compose.
   * ------------------------------------------------------------------ */

  function normalize(str) { return String(str == null ? '' : str).toLowerCase(); }

  function compareByDateAsc(a, b) {
    if (a.date < b.date) return -1;
    if (a.date > b.date) return 1;
    return 0;
  }

  function filterAndSort(transactions, filters) {
    filters = filters || {};
    var search = normalize(filters.search).trim();
    var type = filters.type || 'all';
    var category = filters.category || 'all';
    var sort = filters.sort || 'newest';

    var filtered = (transactions || []).filter(function (tx) {
      if (type !== 'all' && tx.type !== type) return false;
      if (category !== 'all' && tx.category !== category) return false;
      if (search && normalize(tx.description + ' ' + tx.category).indexOf(search) === -1) return false;
      return true;
    });

    filtered.sort(function (a, b) {
      var cmp = 0;
      switch (sort) {
        case 'oldest':      cmp = compareByDateAsc(a, b); break;
        case 'amount-desc': cmp = b.amount - a.amount; break;
        case 'amount-asc':  cmp = a.amount - b.amount; break;
        case 'category-az': cmp = a.category.localeCompare(b.category); break;
        default:            cmp = compareByDateAsc(b, a); /* newest first */
      }
      if (cmp !== 0) return cmp;
      return (a.createdAt || 0) - (b.createdAt || 0); // stable tie-break
    });

    return filtered;
  }

  /* ------------------------------------------------------------------ *
   * Exports
   * ------------------------------------------------------------------ */

  var api = {
    CATEGORIES: CATEGORIES,
    ALL_CATEGORIES: ALL_CATEGORIES,
    toISODate: toISODate,
    currentISODate: currentISODate,
    currentMonthKey: currentMonthKey,
    roundMoney: roundMoney,
    isValidISODate: isValidISODate,
    createSeedTransactions: createSeedTransactions,
    sanitizeTransactions: sanitizeTransactions,
    makeId: makeId,
    validateTransaction: validateTransaction,
    computeTotals: computeTotals,
    computeMonthlySpending: computeMonthlySpending,
    budgetStats: budgetStats,
    filterAndSort: filterAndSort
  };

  global.LedgerlyCore = api;
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
})(typeof window !== 'undefined' ? window : globalThis);
