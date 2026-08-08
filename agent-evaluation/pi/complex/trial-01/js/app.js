/*!
 * Ledgerly — application controller.
 * Owns application state, subscribes to user events, and orchestrates the
 * core (logic) + storage (persistence) + ui (view) layers.
 */
(function () {
  'use strict';

  var state = {
    transactions: [],
    budget: null,
    filters: { search: '', type: 'all', category: 'all', sort: 'newest' },
    theme: 'light'
  };

  var els = {};

  /* ------------------------------------------------------------------ *
   * Element cache + wiring
   * ------------------------------------------------------------------ */

  function cacheElements() {
    els.form = document.getElementById('transaction-form');
    els.description = document.getElementById('description');
    els.amount = document.getElementById('amount');
    els.type = document.getElementById('type');
    els.category = document.getElementById('category');
    els.date = document.getElementById('date');
    els.formError = document.getElementById('form-error');

    els.search = document.getElementById('search');
    els.typeFilter = document.getElementById('type-filter');
    els.categoryFilter = document.getElementById('category-filter');
    els.sortBy = document.getElementById('sort-by');
    els.resetFilters = document.getElementById('reset-filters');

    els.budgetInput = document.getElementById('budget-input');
    els.saveBudget = document.getElementById('save-budget');
    els.budgetMessage = document.getElementById('budget-message');
    els.budgetStatus = document.getElementById('budget-status');
    els.budgetBar = document.getElementById('budget-bar');
    els.budgetProgress = document.getElementById('budget-progress');

    els.tableWrap = document.getElementById('table-wrap');
    els.tbody = document.getElementById('transaction-list');
    els.emptyState = document.getElementById('empty-state');
    els.emptyStateTitle = document.getElementById('empty-state-title');
    els.emptyStateText = document.getElementById('empty-state-text');
    els.emptyStateReset = document.getElementById('empty-state-reset');
    els.resultsMeta = document.getElementById('results-meta');

    els.themeToggle = document.getElementById('theme-toggle');
  }

  function wireEvents() {
    els.form.addEventListener('submit', handleSubmit);

    // Delegate delete clicks from the (re-rendered) table body.
    els.tbody.addEventListener('click', handleDeleteClick);

    // Filters compose: each change re-derives the visible set.
    els.search.addEventListener('input', function () {
      state.filters.search = els.search.value;
      renderTransactionList();
    });
    els.typeFilter.addEventListener('change', function () {
      state.filters.type = els.typeFilter.value;
      renderTransactionList();
    });
    els.categoryFilter.addEventListener('change', function () {
      state.filters.category = els.categoryFilter.value;
      renderTransactionList();
    });
    els.sortBy.addEventListener('change', function () {
      state.filters.sort = els.sortBy.value;
      renderTransactionList();
    });
    els.resetFilters.addEventListener('click', resetFilters);
    els.emptyStateReset.addEventListener('click', resetFilters);

    // The form's category list follows the chosen type.
    els.type.addEventListener('change', function () {
      updateFormCategories(els.type.value);
    });

    els.saveBudget.addEventListener('click', handleSaveBudget);
    els.budgetInput.addEventListener('keydown', function (e) {
      if (e.key === 'Enter') { e.preventDefault(); handleSaveBudget(); }
    });

    els.themeToggle.addEventListener('click', toggleTheme);
  }

  /* ------------------------------------------------------------------ *
   * Rendering
   * ------------------------------------------------------------------ */

  function renderAll() {
    renderSummary();
    renderTransactionList();
    renderBudget();
  }

  function renderSummary() {
    LedgerlyUI.renderSummary(LedgerlyCore.computeTotals(state.transactions));
  }

  function renderTransactionList() {
    var filtered = LedgerlyCore.filterAndSort(state.transactions, state.filters);
    els.tbody.innerHTML = '';

    if (state.transactions.length === 0) {
      showEmptyState('none');
      els.resultsMeta.textContent = 'No transactions recorded';
      return;
    }
    if (filtered.length === 0) {
      showEmptyState('filtered');
      els.resultsMeta.textContent = 'Showing 0 of ' + state.transactions.length + ' transactions';
      return;
    }

    hideEmptyState();
    filtered.forEach(function (tx) {
      els.tbody.appendChild(LedgerlyUI.buildTransactionRow(tx));
    });
    var total = state.transactions.length;
    els.resultsMeta.textContent = 'Showing ' + filtered.length + ' of ' + total + ' transaction' + (total === 1 ? '' : 's');
  }

  function showEmptyState(mode) {
    els.tableWrap.hidden = true;
    els.emptyState.hidden = false;
    if (mode === 'filtered') {
      els.emptyStateTitle.textContent = 'No matching transactions';
      els.emptyStateText.textContent = 'Your search and filters match nothing yet. Try clearing them or adjusting your criteria.';
      els.emptyStateReset.hidden = false;
    } else {
      els.emptyStateTitle.textContent = 'No transactions yet';
      els.emptyStateText.textContent = 'Add your first income or expense using the form above to see your balance here.';
      els.emptyStateReset.hidden = true;
    }
  }

  function hideEmptyState() {
    els.tableWrap.hidden = false;
    els.emptyState.hidden = true;
  }

  function renderBudget() {
    var stats = LedgerlyCore.budgetStats(state.transactions, state.budget);
    LedgerlyUI.renderBudget(els, stats, state.budget);
  }

  /* ------------------------------------------------------------------ *
   * Form handling
   * ------------------------------------------------------------------ */

  function handleSubmit(e) {
    e.preventDefault();

    var result = LedgerlyCore.validateTransaction({
      description: els.description.value,
      amount: els.amount.value,
      type: els.type.value,
      category: els.category.value,
      date: els.date.value
    });

    if (!result.valid) {
      els.formError.textContent = result.errors.join(' ');
      els.formError.hidden = false;
      focusFirstInvalidField(result);
      return;
    }

    var tx = result.transaction;
    tx.id = LedgerlyCore.makeId();
    tx.createdAt = Date.now();

    state.transactions.push(tx);
    LedgerlyStorage.saveTransactions(state.transactions);

    els.formError.hidden = true;
    els.formError.textContent = '';
    els.form.reset();
    els.date.value = LedgerlyCore.currentISODate();
    updateFormCategories(els.type.value);

    renderAll();
    LedgerlyUI.announce('Transaction added: ' + tx.description);
    els.description.focus();
  }

  function focusFirstInvalidField(result) {
    // Route focus to the control behind the first reported error message,
    // so keyboard + screen-reader users land on what needs fixing.
    var err = String((result && result.errors && result.errors[0]) || '');
    if (/description/i.test(err)) { els.description.focus(); }
    else if (/amount/i.test(err)) { els.amount.focus(); }
    else if (/date/i.test(err)) { els.date.focus(); }
    else { els.category.focus(); }
  }

  /* ------------------------------------------------------------------ *
   * Delete handling (event delegation)
   * ------------------------------------------------------------------ */

  function handleDeleteClick(e) {
    var button = e.target.closest('.delete-transaction');
    if (!button) return;

    var id = button.dataset.deleteId;
    var renderedRows = Array.prototype.slice.call(els.tbody.querySelectorAll('.transaction-row'));
    var removedIndex = renderedRows.findIndex(function (row) { return row.dataset.id === id; });
    var removed = state.transactions.find(function (tx) { return tx.id === id; });
    if (!removed) return;

    state.transactions = state.transactions.filter(function (tx) { return tx.id !== id; });
    LedgerlyStorage.saveTransactions(state.transactions);

    renderAll();
    LedgerlyUI.announce('Transaction deleted: ' + removed.description);

    // Move focus to the row now occupying the deleted position (or the
    // empty-state controls / search field if there is nothing left).
    var rows = els.tbody.querySelectorAll('.transaction-row');
    var targetRow = rows[Math.min(removedIndex, rows.length - 1)];
    if (targetRow) {
      var nextDelete = targetRow.querySelector('.delete-transaction');
      if (nextDelete) nextDelete.focus();
    } else if (!els.emptyState.hidden && !els.emptyStateReset.hidden) {
      els.emptyStateReset.focus();
    } else {
      els.search.focus();
    }
  }

  /* ------------------------------------------------------------------ *
   * Filters
   * ------------------------------------------------------------------ */

  function resetFilters() {
    els.search.value = '';
    els.typeFilter.value = 'all';
    els.categoryFilter.value = 'all';
    els.sortBy.value = 'newest';
    state.filters = { search: '', type: 'all', category: 'all', sort: 'newest' };
    renderTransactionList();
    LedgerlyUI.announce('Filters reset');
    els.search.focus();
  }

  function updateFormCategories(type) {
    LedgerlyUI.updateFormCategories(els.category, type);
  }

  /* ------------------------------------------------------------------ *
   * Budget
   * ------------------------------------------------------------------ */

  function handleSaveBudget() {
    var raw = els.budgetInput.value.trim();
    clearBudgetMessageClasses();

    if (raw === '') {
      setBudgetMessage('Please enter a budget amount.', true);
      return;
    }
    var value = Number(raw);
    if (!isFinite(value) || value < 0) {
      setBudgetMessage('Budget must be a number greater than or equal to zero.', true);
      return;
    }

    state.budget = LedgerlyCore.roundMoney(value);
    LedgerlyStorage.saveBudget(state.budget);

    if (state.budget === 0) {
      setBudgetMessage('Budget set to $0 — tracking paused. Enter a larger amount to resume.', false);
    } else {
      setBudgetMessage('Monthly budget saved: ' + LedgerlyUI.formatCurrency(state.budget) + '.', false);
    }
    renderBudget();
    LedgerlyUI.announce(state.budget === 0 ? 'Budget set to zero, tracking paused' : 'Monthly budget saved');
  }

  function setBudgetMessage(text, isError) {
    els.budgetMessage.textContent = text;
    els.budgetMessage.classList.toggle('is-error', isError);
    els.budgetMessage.classList.toggle('is-success', !isError);
    els.budgetMessage.hidden = false;
  }

  function clearBudgetMessageClasses() {
    els.budgetMessage.classList.remove('is-error', 'is-success');
    els.budgetMessage.hidden = true;
  }

  /* ------------------------------------------------------------------ *
   * Theme
   * ------------------------------------------------------------------ */

  function applyTheme(theme) {
    state.theme = theme === 'dark' ? 'dark' : 'light';
    document.documentElement.setAttribute('data-theme', state.theme);
    var isDark = state.theme === 'dark';
    els.themeToggle.setAttribute('aria-pressed', String(isDark));
    var label = isDark ? 'Switch to light theme' : 'Switch to dark theme';
    els.themeToggle.setAttribute('aria-label', label);
    els.themeToggle.setAttribute('title', label);
  }

  function toggleTheme() {
    var next = state.theme === 'dark' ? 'light' : 'dark';
    applyTheme(next);
    LedgerlyStorage.saveTheme(next);
    LedgerlyUI.announce('Theme switched to ' + next);
  }

  /* ------------------------------------------------------------------ *
   * Bootstrap
   * ------------------------------------------------------------------ */

  function init() {
    cacheElements();

    // Saved theme wins; otherwise the head script already applied the
    // system preference, so just mirror it into state.
    applyTheme(LedgerlyStorage.loadTheme() || document.documentElement.getAttribute('data-theme') || 'light');

    state.transactions = LedgerlyStorage.loadTransactions();
    state.budget = LedgerlyStorage.loadBudget();

    // Defaults for first-time visitors. "All categories" maps to value "all"
    // so resetFilters/filterAndSort treat it as "no category filter".
    els.date.value = LedgerlyCore.currentISODate();
    updateFormCategories(els.type.value);
    LedgerlyUI.setSelectOptions(els.categoryFilter, [{ value: 'all', label: 'All categories' }].concat(LedgerlyCore.ALL_CATEGORIES));

    if (state.budget != null) {
      els.budgetInput.value = state.budget;
      if (state.budget === 0) {
        setBudgetMessage('Budget set to $0 — tracking paused. Enter a larger amount to resume.', false);
      } else {
        setBudgetMessage('Monthly budget saved: ' + LedgerlyUI.formatCurrency(state.budget) + '.', false);
      }
    }

    wireEvents();
    renderAll();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
