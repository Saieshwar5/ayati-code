/*!
 * Ledgerly — view layer.
 * Rendering helpers (pure DOM construction + formatting). No state, no event
 * wiring; app.js owns state and calls into here to paint the screen.
 */
(function (global) {
  'use strict';

  var currencyFmt = new Intl.NumberFormat('en-US', { style: 'currency', currency: 'USD' });
  var dateFmt = new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric', year: 'numeric' });

  function $(sel, root) { return (root || document).querySelector(sel); }

  function formatCurrency(n) { return currencyFmt.format(n); }

  function formatDate(iso) {
    var p = String(iso).split('-').map(Number);
    if (p.length !== 3 || p.some(isNaN)) return iso;
    return dateFmt.format(new Date(p[0], p[1] - 1, p[2]));
  }

  /* Small inline icons (no external assets). */
  var TRASH_ICON =
    '<svg viewBox="0 0 24 24" width="16" height="16" aria-hidden="true" focusable="false">' +
    '<path d="M3 6h18M8 6V4h8v2m1 0-1 14H8L7 6M10 11v6M14 11v6" fill="none" stroke="currentColor" ' +
    'stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>';

  /* ------------------------------------------------------------------ *
   * Transaction row
   * ------------------------------------------------------------------ */

  function buildTransactionRow(tx) {
    var tr = document.createElement('tr');
    tr.className = 'transaction-row';
    tr.dataset.id = tx.id;

    var descCell = document.createElement('td');
    descCell.className = 'transaction-desc';
    descCell.textContent = tx.description;
    descCell.setAttribute('data-label', 'Description');

    var catCell = document.createElement('td');
    catCell.className = 'transaction-category';
    var catBadge = document.createElement('span');
    catBadge.className = 'badge';
    catBadge.textContent = tx.category;
    catCell.appendChild(catBadge);
    catCell.setAttribute('data-label', 'Category');

    var dateCell = document.createElement('td');
    dateCell.className = 'transaction-date';
    dateCell.textContent = formatDate(tx.date);
    dateCell.setAttribute('data-label', 'Date');

    var typeCell = document.createElement('td');
    typeCell.className = 'transaction-type';
    var typeBadge = document.createElement('span');
    typeBadge.className = 'badge badge--' + tx.type;
    typeBadge.textContent = tx.type === 'income' ? 'Income' : 'Expense';
    typeCell.appendChild(typeBadge);
    typeCell.setAttribute('data-label', 'Type');

    var amountCell = document.createElement('td');
    amountCell.className = 'transaction-amount amount--' + tx.type + ' align-right';
    amountCell.textContent = (tx.type === 'income' ? '+' : '\u2212') + formatCurrency(tx.amount);
    amountCell.setAttribute('data-label', 'Amount');

    var actionCell = document.createElement('td');
    actionCell.className = 'transaction-actions align-right';
    var del = document.createElement('button');
    del.type = 'button';
    del.className = 'delete-transaction';
    del.dataset.deleteId = tx.id;
    del.setAttribute('aria-label', 'Delete ' + tx.description);
    del.setAttribute('title', 'Delete transaction');
    del.innerHTML = TRASH_ICON;
    actionCell.appendChild(del);

    tr.appendChild(descCell);
    tr.appendChild(catCell);
    tr.appendChild(dateCell);
    tr.appendChild(typeCell);
    tr.appendChild(amountCell);
    tr.appendChild(actionCell);

    return tr;
  }

  /* ------------------------------------------------------------------ *
   * Summary cards
   * ------------------------------------------------------------------ */

  function renderSummary(totals) {
    var balanceEl = $('#summary-balance');
    balanceEl.textContent = formatCurrency(totals.balance);
    balanceEl.classList.toggle('is-negative', totals.balance < 0);

    $('#summary-income').textContent = formatCurrency(totals.income);
    $('#summary-expense').textContent = formatCurrency(totals.expenses);
    $('#summary-income-count').textContent = totals.incomeCount + (totals.incomeCount === 1 ? ' transaction' : ' transactions');
    $('#summary-expense-count').textContent = totals.expenseCount + (totals.expenseCount === 1 ? ' transaction' : ' transactions');
  }

  /* ------------------------------------------------------------------ *
   * Selects
   * ------------------------------------------------------------------ */

  /* Replaces a <select>'s options, preserving selection when still valid.
     Entries may be plain strings (value === label) or { value, label } pairs. */
  function setSelectOptions(select, options) {
    var current = select.value;
    var values = options.map(function (opt) {
      return typeof opt === 'object' && opt !== null ? opt.value : opt;
    });
    select.innerHTML = '';
    values.forEach(function (value, i) {
      var o = document.createElement('option');
      o.value = value;
      o.textContent = typeof options[i] === 'object' && options[i] !== null ? options[i].label : value;
      select.appendChild(o);
    });
    if (values.indexOf(current) !== -1) select.value = current;
  }

  /* Repopulate the form's category dropdown for the chosen type. */
  function updateFormCategories(select, type) {
    setSelectOptions(select, LedgerlyCore.CATEGORIES[type] || []);
  }

  /* ------------------------------------------------------------------ *
   * Budget progress
   * ------------------------------------------------------------------ */

  /**
   * @param {Object} stats  from LedgerlyCore.budgetStats
   * @param {number|null} budget
   */
  function renderBudget(els, stats, budget) {
    var status = els.budgetStatus;
    var bar = els.budgetBar;
    var track = els.budgetProgress;

    track.classList.remove('budget-progress--idle', 'budget-progress--ok', 'budget-progress--warn', 'budget-progress--over');

    if (budget == null) {
      track.classList.add('budget-progress--idle');
      bar.style.width = '0%';
      track.setAttribute('aria-valuenow', '0');
      status.textContent = 'No budget set yet — save a monthly budget above.';
      return;
    }

    if (budget <= 0) {
      // Explicitly saved zero budget: never divide, never show NaN/Infinity.
      track.classList.add('budget-progress--idle');
      bar.style.width = '0%';
      track.setAttribute('aria-valuenow', '0');
      status.textContent = 'Budget is $0 — tracking paused. Set a larger budget to start.';
      return;
    }

    var realPercent = stats.percent;
    var clampedPercent = Math.min(realPercent, 100);
    bar.style.width = clampedPercent + '%';
    track.setAttribute('aria-valuenow', String(Math.round(clampedPercent)));

    if (realPercent > 100) {
      track.classList.add('budget-progress--over');
      status.textContent = 'Overspent by ' + formatCurrency(stats.over) + ' — ' +
        formatCurrency(stats.spent) + ' spent of ' + formatCurrency(budget) + ' (' + Math.round(realPercent) + '%).';
    } else if (realPercent >= 80) {
      track.classList.add('budget-progress--warn');
      status.textContent = formatCurrency(stats.spent) + ' of ' + formatCurrency(budget) + ' used (' +
        Math.round(realPercent) + '%). Getting close to the limit.';
    } else {
      track.classList.add('budget-progress--ok');
      status.textContent = formatCurrency(stats.spent) + ' of ' + formatCurrency(budget) + ' used (' +
        Math.round(realPercent) + '%).';
    }
  }

  /* ------------------------------------------------------------------ *
   * Announcements for assistive tech (polite live region).
   * ------------------------------------------------------------------ */

  var announceTimer = null;
  function announce(message) {
    var region = $('#announcer');
    if (!region) return;
    // A new announcement cancels the pending clear of the previous one, so
    // rapid successive announcements are never wiped by a stale timer.
    if (announceTimer) { window.clearTimeout(announceTimer); announceTimer = null; }
    region.textContent = '';
    window.setTimeout(function () {
      region.textContent = message;
      announceTimer = window.setTimeout(function () {
        region.textContent = '';
        announceTimer = null;
      }, 4000);
    }, 30);
  }

  var api = {
    $: $,
    formatCurrency: formatCurrency,
    formatDate: formatDate,
    buildTransactionRow: buildTransactionRow,
    renderSummary: renderSummary,
    setSelectOptions: setSelectOptions,
    updateFormCategories: updateFormCategories,
    renderBudget: renderBudget,
    announce: announce
  };

  global.LedgerlyUI = api;
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
})(typeof window !== 'undefined' ? window : globalThis);
