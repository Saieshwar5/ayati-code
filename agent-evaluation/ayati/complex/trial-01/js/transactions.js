/* ============================================================
   Ledgerly — transaction model, seed data, and rendering.
   ============================================================ */

const Transactions = (() => {
  const CATEGORIES = [
    'Food & Dining', 'Transport', 'Housing', 'Utilities',
    'Entertainment', 'Shopping', 'Health', 'Salary', 'Freelance', 'Other'
  ];

  // Realistic seed data, used only when no saved data exists.
  function seedData() {
    const today = new Date();
    const daysAgo = (n) => {
      const d = new Date(today);
      d.setDate(d.getDate() - n);
      return d.toISOString().slice(0, 10);
    };
    return [
      { id: uid(), description: 'Monthly salary', amount: 4200, type: 'income', category: 'Salary', date: daysAgo(2) },
      { id: uid(), description: 'Freelance design project', amount: 650, type: 'income', category: 'Freelance', date: daysAgo(6) },
      { id: uid(), description: 'Rent payment', amount: 1400, type: 'expense', category: 'Housing', date: daysAgo(1) },
      { id: uid(), description: 'Weekly groceries', amount: 86.4, type: 'expense', category: 'Food & Dining', date: daysAgo(3) },
      { id: uid(), description: 'Electricity bill', amount: 74.2, type: 'expense', category: 'Utilities', date: daysAgo(4) },
      { id: uid(), description: 'Gas refill', amount: 45, type: 'expense', category: 'Transport', date: daysAgo(5) },
      { id: uid(), description: 'Movie night', amount: 28.5, type: 'expense', category: 'Entertainment', date: daysAgo(7) }
    ];
  }

  function uid() {
    return Date.now().toString(36) + Math.random().toString(36).slice(2, 8);
  }

  function load() {
    const saved = Storage.getTransactions();
    if (Array.isArray(saved) && saved.length >= 0) {
      return saved;
    }
    const seeded = seedData();
    Storage.saveTransactions(seeded);
    return seeded;
  }

  function formatCurrency(value) {
    return new Intl.NumberFormat('en-US', {
      style: 'currency',
      currency: 'USD'
    }).format(value);
  }

  function formatDate(iso) {
    if (!iso) return '—';
    const d = new Date(iso + 'T00:00:00');
    if (Number.isNaN(d.getTime())) return iso;
    return d.toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
  }

  function renderRow(tx) {
    const tr = document.createElement('tr');
    tr.className = 'transaction-row';
    tr.dataset.id = tx.id;

    const amountClass = tx.type === 'income' ? 'amount-income' : 'amount-expense';
    const sign = tx.type === 'income' ? '+' : '−';

    tr.innerHTML = `
      <td class="desc-cell">${escapeHtml(tx.description)}</td>
      <td><span class="category-badge">${escapeHtml(tx.category)}</span></td>
      <td class="date-cell">${escapeHtml(formatDate(tx.date))}</td>
      <td class="col-amount ${amountClass}">${sign}${formatCurrency(tx.amount)}</td>
      <td class="col-actions">
        <button type="button" class="btn btn-danger delete-transaction" aria-label="Delete ${escapeHtml(tx.description)}">Delete</button>
      </td>
    `;
    return tr;
  }

  function escapeHtml(str) {
    return String(str)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  return {
    CATEGORIES,
    load,
    uid,
    formatCurrency,
    formatDate,
    renderRow,
    escapeHtml
  };
})();
