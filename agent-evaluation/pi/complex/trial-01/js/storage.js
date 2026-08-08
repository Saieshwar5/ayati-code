/*!
 * Ledgerly — persistence layer.
 * Thin, defensive wrapper around localStorage. Every read/write is wrapped in
 * try/catch so private mode, full disks, or blocked storage can never crash
 * the dashboard. Seed data is written only when no saved data exists.
 */
(function (global) {
  'use strict';

  var KEYS = {
    TRANSACTIONS: 'ledgerly-transactions',
    BUDGET: 'ledgerly-budget',
    THEME: 'ledgerly-theme'
  };

  function loadRaw(key) {
    try { return localStorage.getItem(key); } catch (e) { return null; }
  }

  function saveRaw(key, value) {
    try { localStorage.setItem(key, value); return true; } catch (e) {
      if (global.console && console.warn) console.warn('Ledgerly: unable to persist "' + key + '"', e);
      return false;
    }
  }

  /* ------------------------------------------------------------------ *
   * Transactions
   * ------------------------------------------------------------------ */

  function loadTransactions() {
    var raw = loadRaw(KEYS.TRANSACTIONS);
    if (raw === null) {
      // Nothing saved yet: seed once, then persist the seed.
      var seed = LedgerlyCore.createSeedTransactions();
      saveRaw(KEYS.TRANSACTIONS, JSON.stringify(seed));
      return seed;
    }
    try {
      var parsed = JSON.parse(raw);
      if (!Array.isArray(parsed)) return [];
      // Sanitize: drop malformed/duplicate entries instead of letting them
      // reach the renderer, totals, or budget math — and persist the cleaned
      // list so garbage does not resurface on every subsequent load.
      var sanitized = LedgerlyCore.sanitizeTransactions(parsed);
      if (sanitized.length !== parsed.length) {
        saveRaw(KEYS.TRANSACTIONS, JSON.stringify(sanitized));
      }
      return sanitized;
    } catch (e) {
      // Corrupt payload: rebuild from seed rather than surfacing a crash.
      var fresh = LedgerlyCore.createSeedTransactions();
      saveRaw(KEYS.TRANSACTIONS, JSON.stringify(fresh));
      return fresh;
    }
  }

  function saveTransactions(transactions) {
    return saveRaw(KEYS.TRANSACTIONS, JSON.stringify(transactions));
  }

  /* ------------------------------------------------------------------ *
   * Budget (stored as a plain number in JSON, never a string);
   * null = unset, 0 = explicitly saved "no budget".
   * ------------------------------------------------------------------ */

  function loadBudget() {
    var raw = loadRaw(KEYS.BUDGET);
    if (raw === null) return null;
    var value = Number(raw);
    return isFinite(value) && value >= 0 ? value : null;
  }

  function saveBudget(budget) {
    return saveRaw(KEYS.BUDGET, JSON.stringify(budget));
  }

  /* ------------------------------------------------------------------ *
   * Theme (stored as a bare string so the head script can read it
   * synchronously before first paint).
   * ------------------------------------------------------------------ */

  function loadTheme() {
    var raw = loadRaw(KEYS.THEME);
    return raw === 'dark' || raw === 'light' ? raw : null;
  }

  function saveTheme(theme) {
    saveRaw(KEYS.THEME, theme);
  }

  var api = {
    KEYS: KEYS,
    loadTransactions: loadTransactions,
    saveTransactions: saveTransactions,
    loadBudget: loadBudget,
    saveBudget: saveBudget,
    loadTheme: loadTheme,
    saveTheme: saveTheme
  };

  global.LedgerlyStorage = api;
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
})(typeof window !== 'undefined' ? window : globalThis);
