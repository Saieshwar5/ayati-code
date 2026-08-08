/* ============================================================
   Ledgerly — application controller.
   Wires up the form, filters, sorting, budget, and rendering.
   ============================================================ */

const App = (() => {
  let transactions = [];
  let budget = null;

  const els = {
    form: document.getElementById('transaction-form'),
    formError: document.getElementById('form-error'),
    description: document.getElementById('description'),
    amount: document.getElementById('amount'),
    type: document.getElementById('type'),
    category: document.getElementById('category'),
    date: document.getElementById('date'),
    tbody: document.getElementById('transactions-body'),
    emptyState: document.getElementById('empty-state'),
    emptyText: document.getElementById('empty-text'),
    search: document.getElementById('search'),
    typeFilter: document.getElementById('type-filter'),
    categoryFilter: document.getElementById('category-filter'),
    sortBy: document.getElementById('sort-by'),
    resetFilters: document.getElementById('reset-filters'),
    summaryBalance: document.getElementById('summary-balance'),
    summaryIncome: document.getElementById('summary-income'),
    summaryExpenses: document.getElementById('summary-expenses'),
    budgetInput: document.getElementById('budget-input'),
    saveBudget: document.getElementById('save-budget'),
    budgetFill: document.getElementById('budget-progress-fill'),
    budgetStatus: document.getElementById('budget-status'),
    budgetBar: document.querySelector('.budget-progress')
  };

  /* ---------- Init ---------- */
  function init() {
    transactions = Transactions.load();
    budget = Storage.getBudget();
    populateCategoryFilter();
    populateFormCategories();
    if (budget !== null) els.budgetInput.value = budget;
    bindEvents();
    render();
  }

  function bindEvents() {
    els.form.addEventListener('submit', handleSubmit);
    els.tbody.addEventListener('click', handleDelete);
    els.search.addEventListener('input', render);
    els.typeFilter.addEventListener('change', render);
    els.categoryFilter.addEventListener('change', render);
    els.sortBy.addEventListener('change', render);
    els.resetFilters.addEventListener('click', resetFilters);
    els.saveBudget.addEventListener('click', saveBudget);
    els.budgetInput.addEventListener('keydown', (e) => {
      if (e.key === 'Enter') { e.preventDefault(); saveBudget(); }
    });
  }

  /* ---------- Category options ---------- */
  function populateCategoryFilter() {
    const frag = document.createDocumentFragment();
    const all = document.createElement('option');
    all.value = 'all';
    all.textContent = 'All categories';
    frag.appendChild(all);
    Transactions.CATEGORIES.forEach((cat) => {
      const opt = document.createElement('option');
      opt.value = cat;
      opt.textContent = cat;
      frag.appendChild(opt);
    });
    els.categoryFilter.appendChild(frag);
  }

  function populateFormCategories() {
    // Form select already has static options; keep in sync with CATEGORIES.
    const current = els.category.value;
    els.category.innerHTML = '';
    Transactions.CATEGORIES.forEach((cat) => {
      const opt = document.createElement('option');
      opt.value = cat;
      opt.textContent = cat;
      els.category.appendChild(opt);
    });
    if (Transactions.CATEGORIES.includes(current)) els.category.value = current;
  }

  /* ---------- Add transaction ---------- */
  function handleSubmit(e) {
    e.preventDefault();
    els.formError.textContent = '';

    const description = els.description.value.trim();
    const amountRaw = els.amount.value.trim();
    const type = els.type.value;
    const category = els.category.value;
    const date = els.date.value;

    if (!description) {
      showError('Please enter a description.');
      els.description.focus();
      return;
    }
    const amount = Number(amountRaw);
    if (amountRaw === '' || !Number.isFinite(amount) || amount <= 0) {
      showError('Please enter a valid amount greater than zero.');
      els.amount.focus();
      return;
    }
    if (!date) {
      showError('Please choose a date.');
      els.date.focus();
      return;
    }

    const tx = {
      id: Transactions.uid(),
      description,
      amount: Math.round(amount * 100) / 100,
      type,
      category,
      date
    };

    transactions.push(tx);
    Storage.saveTransactions(transactions);
    els.form.reset();
    els.type.value = 'expense';
    els.category.value = 'Food & Dining';
    els.description.focus();
    render();
  }

  function showError(msg) {
    els.formError.textContent = msg;
  }

  /* ---------- Delete transaction ---------- */
  function handleDelete(e) {
    const btn = e.target.closest('.delete-transaction');
    if (!btn) return;
    const row = btn.closest('.transaction-row');
    if (!row) return;
    const id = row.dataset.id;
    transactions = transactions.filter((t) => t.id !== id);
    Storage.saveTransactions(transactions);
    render();
  }

  /* ---------- Filtering & sorting ---------- */
  function getFiltered() {
    const q = els.search.value.trim().toLowerCase();
    const type = els.typeFilter.value;
    const category = els.categoryFilter.value;

    let list = transactions.filter((t) => {
      if (type !== 'all' && t.type !== type) return false;
      if (category !== 'all' && t.category !== category) return false;
      if (q && !t.description.toLowerCase().includes(q)) return false;
      return true;
    });

    const sort = els.sortBy.value;
    list = list.slice().sort((a, b) => {
      switch (sort) {
        case 'date-asc':
          return a.date.localeCompare(b.date);
        case 'amount-desc':
          return b.amount - a.amount;
        case 'amount-asc':
          return a.amount - b.amount;
        case 'date-desc':
        default:
          return b.date.localeCompare(a.date);
      }
    });
    return list;
  }

  function resetFilters() {
    els.search.value = '';
    els.typeFilter.value = 'all';
    els.categoryFilter.value = 'all';
    els.sortBy.value = 'date-desc';
    render();
  }

  /* ---------- Budget ---------- */
  function saveBudget() {
    const raw = els.budgetInput.value.trim();
    if (raw === '') {
      budget = null;
      Storage.saveBudget(null);
      els.budgetInput.value = '';
      render();
      return;
    }
    const value = Number(raw);
    if (!Number.isFinite(value) || value < 0) {
      els.budgetStatus.textContent = 'Please enter a valid budget amount (0 or more).';
      els.budgetInput.focus();
      return;
    }
    budget = Math.round(value * 100) / 100;
    Storage.saveBudget(budget);
    render();
  }

  function renderBudget() {
    const totalExpenses = transactions
      .filter((t) => t.type === 'expense')
      .reduce((sum, t) => sum + t.amount, 0);

    if (budget === null) {
      els.budgetFill.style.width = '0%';
      els.budgetFill.classList.remove('warn', 'over');
      els.budgetBar.setAttribute('aria-valuenow', '0');
      els.budgetStatus.innerHTML = 'No budget set yet. Set a monthly budget to track your spending.';
      return;
    }

    if (budget === 0) {
      els.budgetFill.style.width = '0%';
      els.budgetFill.classList.remove('warn', 'over');
      els.budgetBar.setAttribute('aria-valuenow', '0');
      els.budgetStatus.innerHTML =
        `Budget is <strong>${Transactions.formatCurrency(0)}</strong>. ` +
        `Spent <strong>${Transactions.formatCurrency(totalExpenses)}</strong>. ` +
        'Set a budget above zero to see progress.';
      return;
    }

    const pct = Math.min((totalExpenses / budget) * 100, 100);
    els.budgetFill.style.width = pct + '%';
    els.budgetBar.setAttribute('aria-valuenow', String(Math.round(pct)));
    els.budgetFill.classList.toggle('warn', pct >= 75 && pct < 100);
    els.budgetFill.classList.toggle('over', pct >= 100);

    const remaining = budget - totalExpenses;
    let message;
    if (remaining < 0) {
      message = `You are <strong>${Transactions.formatCurrency(Math.abs(remaining))}</strong> over your ` +
        `<strong>${Transactions.formatCurrency(budget)}</strong> budget.`;
    } else {
      message = `Spent <strong>${Transactions.formatCurrency(totalExpenses)}</strong> of ` +
        `<strong>${Transactions.formatCurrency(budget)}</strong> budget ` +
        `(${Math.round(pct)}%). ${Transactions.formatCurrency(remaining)} remaining.`;
    }
    els.budgetStatus.innerHTML = message;
  }

  /* ---------- Summary ---------- */
  function renderSummary() {
    const income = transactions
      .filter((t) => t.type === 'income')
      .reduce((s, t) => s + t.amount, 0);
    const expenses = transactions
      .filter((t) => t.type === 'expense')
      .reduce((s, t) => s + t.amount, 0);
    const balance = income - expenses;

    els.summaryIncome.textContent = Transactions.formatCurrency(income);
    els.summaryExpenses.textContent = Transactions.formatCurrency(expenses);
    els.summaryBalance.textContent = Transactions.formatCurrency(balance);
  }

  /* ---------- Render ---------- */
  function render() {
    renderSummary();
    renderBudget();

    const list = getFiltered();
    els.tbody.innerHTML = '';

    if (list.length === 0) {
      els.emptyState.hidden = false;
      const hasAny = transactions.length > 0;
      els.emptyText.textContent = hasAny
        ? 'No transactions match your current filters. Try resetting them.'
        : 'Add your first transaction to get started.';
      return;
    }

    els.emptyState.hidden = true;
    const frag = document.createDocumentFragment();
    list.forEach((tx) => frag.appendChild(Transactions.renderRow(tx)));
    els.tbody.appendChild(frag);
  }

  return { init };
})();

document.addEventListener('DOMContentLoaded', () => {
  Theme.init();
  App.init();
});
