/* ============================================================
   Ledgerly — storage layer
   Thin wrapper around localStorage with safe JSON parsing.
   ============================================================ */

const Storage = (() => {
  const KEYS = {
    transactions: 'ledgerly-transactions',
    budget: 'ledgerly-budget',
    theme: 'ledgerly-theme'
  };

  function read(key, fallback) {
    try {
      const raw = localStorage.getItem(key);
      if (raw === null) return fallback;
      return JSON.parse(raw);
    } catch (err) {
      console.warn(`Ledgerly: could not read "${key}", using fallback.`, err);
      return fallback;
    }
  }

  function write(key, value) {
    try {
      localStorage.setItem(key, JSON.stringify(value));
      return true;
    } catch (err) {
      console.warn(`Ledgerly: could not write "${key}".`, err);
      return false;
    }
  }

  return {
    KEYS,
    getTransactions: () => read(KEYS.transactions, null),
    saveTransactions: (list) => write(KEYS.transactions, list),
    getBudget: () => read(KEYS.budget, null),
    saveBudget: (value) => write(KEYS.budget, value),
    getTheme: () => read(KEYS.theme, null),
    saveTheme: (value) => write(KEYS.theme, value)
  };
})();
