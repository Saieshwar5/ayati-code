'use strict';
/*
 * Ledgerly — logic test suite (no browser needed).
 * Run from the project root:
 *   node tests/logic.test.js
 *
 * Requires only core.js + storage.js; localStorage is stubbed.
 */
const assert = require('assert');
const path = require('path');

const root = path.join(__dirname, '..');

// --- stub a minimal browser-ish environment ------------------------------
const store = new Map();
global.window = global;
global.localStorage = {
  getItem: (k) => (store.has(k) ? store.get(k) : null),
  setItem: (k, v) => { store.set(k, String(v)); },
  removeItem: (k) => { store.delete(k); },
  clear: () => { store.clear(); }
};

const core = require(path.join(root, 'js', 'core.js'));
const storage = require(path.join(root, 'js', 'storage.js'));

let failures = 0;
function test(name, fn) {
  try { fn(); console.log('  PASS  ' + name); }
  catch (e) { failures++; console.log('  FAIL  ' + name + '\n        ' + (e.message || e)); }
}

console.log('\n== seeds ==');
test('seed has >= 5 realistic, fully-formed transactions', () => {
  const seed = core.createSeedTransactions();
  assert.ok(seed.length >= 5, 'got ' + seed.length);
  seed.forEach(tx => {
    assert.ok(tx.description && tx.description.length > 0);
    assert.ok(tx.amount > 0);
    assert.ok(['income', 'expense'].includes(tx.type));
    assert.ok(core.isValidISODate(tx.date), 'bad date ' + tx.date);
    assert.ok(tx.id && tx.createdAt != null);
  });
});
test('some seeds live in the current month (budget exercises)', () => {
  const seed = core.createSeedTransactions();
  const key = core.currentMonthKey();
  assert.ok(seed.filter(t => t.date.slice(0, 7) === key && t.type === 'expense').length >= 5);
});

console.log('\n== validation ==');
test('rejects empty / whitespace description', () => {
  ['', '   '].forEach(d => {
    const r = core.validateTransaction({ description: d, amount: 10, type: 'expense', category: 'Housing', date: '2025-01-01' });
    assert.ok(!r.valid, 'desc "' + d + '" should fail');
    assert.ok(/description/i.test(r.errors[0]));
  });
});
test('rejects amount 0, negative, NaN, empty', () => {
  ['0', '-5', 'abc', '', '   ', 'NaN'].forEach(a => {
    const r = core.validateTransaction({ description: 'x', amount: a, type: 'expense', category: 'Housing', date: '2025-01-01' });
    assert.ok(!r.valid, 'amount "' + a + '" should fail');
  });
});
test('rejects sub-cent amounts that would round to $0.00', () => {
  ['0.004', '0.001', '0.0001'].forEach(a => {
    const r = core.validateTransaction({ description: 'x', amount: a, type: 'expense', category: 'Housing', date: '2025-01-01' });
    assert.ok(!r.valid, 'amount "' + a + '" should fail');
    assert.ok(/0\.01/i.test(r.errors[0]), 'message should mention $0.01: ' + r.errors[0]);
  });
});
test('accepts amounts that round up to a cent', () => {
  const r = core.validateTransaction({ description: 'x', amount: '0.006', type: 'expense', category: 'Housing', date: '2025-01-01' });
  assert.ok(r.valid, JSON.stringify(r.errors));
  assert.strictEqual(r.transaction.amount, 0.01);
});
test('rejects missing / malformed dates', () => {
  const base = { description: 'x', amount: 5, type: 'expense', category: 'Housing' };
  assert.ok(!core.validateTransaction(Object.assign({}, base, { date: '' })).valid);
  assert.ok(!core.validateTransaction(Object.assign({}, base, { date: '2025-02-30' })).valid);
  assert.ok(!core.validateTransaction(Object.assign({}, base, { date: '13/01/2025' })).valid);
  assert.ok(core.validateTransaction(Object.assign({}, base, { date: '2025-02-28' })).valid);
  assert.ok(core.validateTransaction(Object.assign({}, base, { date: '2024-02-29' })).valid);
});
test('accepts valid transaction and rounds money', () => {
  const r = core.validateTransaction({ description: '  Coffee  ', amount: '4.2395', type: 'income', category: 'Freelance', date: '2025-03-15' });
  assert.ok(r.valid && r.transaction.description === 'Coffee');
  assert.strictEqual(r.transaction.amount, 4.24);
  assert.strictEqual(r.transaction.type, 'income');
});

console.log('\n== sanitizeTransactions ==');
test('drops malformed entries, keeps valid ones in order', () => {
  const dirty = [
    null,
    { id: 'a', description: 'ok', amount: 10, type: 'expense', category: 'Housing', date: '2025-01-02', createdAt: 5 },
    { id: 'b', description: '', amount: 10, type: 'expense', category: 'Housing', date: '2025-01-02' },
    { id: 'c', description: 'bad amount', amount: -3, type: 'expense', category: 'Housing', date: '2025-01-02' },
    { id: 'd', description: 'tiny', amount: 0.004, type: 'expense', category: 'Housing', date: '2025-01-02' },
    { id: 'e', description: 'bad date', amount: 5, type: 'expense', category: 'Housing', date: '2025-02-30' },
    { id: 'f', description: 'bad type defaults to expense', amount: 5, type: 'gift', category: 'Gifts', date: '2025-01-03' },
    { description: 'no id gets one', amount: 7.129, type: 'income', category: 'Salary', date: '2025-01-04' },
    'not-an-object',
    42
  ];
  const clean = core.sanitizeTransactions(dirty);
  assert.strictEqual(clean.length, 3); // a, f, g
  assert.strictEqual(clean[0].id, 'a');
  assert.strictEqual(clean[1].type, 'expense');
  assert.strictEqual(clean[1].category, 'Gifts');
  assert.ok(clean[2].id && /^tx-/.test(clean[2].id), 'missing id should be generated');
  assert.strictEqual(clean[2].amount, 7.13);
  assert.ok(clean[2].createdAt != null);
});
test('dedupes duplicate ids', () => {
  const dup = core.sanitizeTransactions([
    { id: 'x', description: 'one', amount: 1, type: 'expense', category: 'Housing', date: '2025-01-01' },
    { id: 'x', description: 'two', amount: 2, type: 'expense', category: 'Housing', date: '2025-01-01' }
  ]);
  assert.strictEqual(dup.length, 1);
  assert.strictEqual(dup[0].description, 'one');
});
test('non-array input yields empty array', () => {
  assert.deepStrictEqual(core.sanitizeTransactions('nope'), []);
  assert.deepStrictEqual(core.sanitizeTransactions(undefined), []);
});

console.log('\n== storage (stub localStorage) ==');
store.clear();
test('loads seed when nothing saved and persists it', () => {
  const txs = storage.loadTransactions();
  assert.ok(txs.length >= 5);
  assert.ok(store.has('ledgerly-transactions'));
});
test('does NOT reseed when saved list is empty []', () => {
  store.set('ledgerly-transactions', '[]');
  assert.deepStrictEqual(storage.loadTransactions(), []);
});
test('save/load round-trip survives', () => {
  const txs = [{ id: 'a', description: 'd', amount: 1, type: 'income', category: 'Salary', date: '2025-01-01', createdAt: 1 }];
  storage.saveTransactions(txs);
  assert.deepStrictEqual(storage.loadTransactions(), txs);
});
test('corrupt JSON rebuilds from seed', () => {
  store.set('ledgerly-transactions', '{{{not json');
  const recovered = storage.loadTransactions();
  assert.ok(Array.isArray(recovered) && recovered.length >= 5);
});
test('valid-JSON-but-garbage entries are sanitized (no crash, no junk rows)', () => {
  store.set('ledgerly-transactions', JSON.stringify([
    { id: 'a', description: 'Hack', amount: -99, type: 'expense', category: 'X', date: 'bad' },
    { id: 'b', description: 'Legit', amount: '42', type: 'expense', category: 'Housing', date: '2025-01-01', createdAt: 1 }
  ]));
  const txs = storage.loadTransactions();
  assert.strictEqual(txs.length, 1);
  assert.strictEqual(txs[0].id, 'b');
  assert.strictEqual(txs[0].amount, 42);
});
test('budget round-trip incl. explicit 0 and negative-to-null safety', () => {
  store.clear();
  assert.strictEqual(storage.loadBudget(), null);
  storage.saveBudget(0);
  assert.strictEqual(storage.loadBudget(), 0);
  storage.saveBudget(3025.5);
  assert.strictEqual(storage.loadBudget(), 3025.5);
  store.set('ledgerly-budget', '-3');
  assert.strictEqual(storage.loadBudget(), null);
  store.set('ledgerly-budget', 'garbage');
  assert.strictEqual(storage.loadBudget(), null);
});
test('theme round-trip', () => {
  store.clear();
  assert.strictEqual(storage.loadTheme(), null);
  storage.saveTheme('dark');
  assert.strictEqual(storage.loadTheme(), 'dark');
  store.set('ledgerly-theme', 'neon');
  assert.strictEqual(storage.loadTheme(), null);
});
test('blocked localStorage degrades gracefully', () => {
  store.clear();
  global.localStorage = {
    getItem: () => { throw new Error('SecurityError'); },
    setItem: () => { throw new Error('QuotaExceeded'); }
  };
  const txs = storage.loadTransactions();
  assert.ok(txs.length >= 5);
  storage.saveTransactions([]); // must not throw
  storage.saveTheme('dark');
});

console.log('\n== totals ==');
test('computeTotals sums income/expenses/balance correctly', () => {
  const txs = [
    { type: 'income', amount: 1000 }, { type: 'income', amount: 250.5 },
    { type: 'expense', amount: 300 }, { type: 'expense', amount: 12.34 }
  ];
  const t = core.computeTotals(txs);
  assert.strictEqual(t.income, 1250.5);
  assert.strictEqual(t.expenses, 312.34);
  assert.strictEqual(t.balance, 938.16);
  assert.strictEqual(t.incomeCount, 2);
  assert.strictEqual(t.expenseCount, 2);
});

console.log('\n== filtering & sorting (composable) ==');
test('type filter', () => {
  assert.strictEqual(core.filterAndSort(core.createSeedTransactions(), { type: 'income' }).length, 3);
  assert.strictEqual(core.filterAndSort(core.createSeedTransactions(), { type: 'expense' }).length, 7);
});
test('category filter', () => {
  assert.strictEqual(core.filterAndSort(core.createSeedTransactions(), { category: 'Transport' }).length, 2);
});
test('search (case-insensitive, description+category)', () => {
  assert.strictEqual(core.filterAndSort(core.createSeedTransactions(), { search: 'SALARY' }).length, 1);
  assert.strictEqual(core.filterAndSort(core.createSeedTransactions(), { search: 'stream' }).length, 1);
  assert.strictEqual(core.filterAndSort(core.createSeedTransactions(), { search: 'zzzz' }).length, 0);
});
test('combined search + type + category', () => {
  const r = core.filterAndSort(core.createSeedTransactions(), { search: 'insurance', type: 'expense', category: 'Transport' });
  assert.strictEqual(r.length, 1);
  assert.strictEqual(r[0].description, 'Car insurance');
});
test('sorting orders', () => {
  const seed = core.createSeedTransactions();
  const newest = core.filterAndSort(seed, { sort: 'newest' });
  assert.ok(newest[0].date >= newest[newest.length - 1].date);
  const oldest = core.filterAndSort(seed, { sort: 'oldest' });
  assert.ok(oldest[0].date <= oldest[oldest.length - 1].date);
  const hi = core.filterAndSort(seed, { sort: 'amount-desc' });
  assert.strictEqual(hi[0].amount, 5200);
  const lo = core.filterAndSort(seed, { sort: 'amount-asc' });
  assert.strictEqual(lo[0].amount, 12.99);
  const az = core.filterAndSort(seed, { sort: 'category-az' });
  assert.ok(az[0].category <= az[az.length - 1].category);
});
test('filter does not mutate the source array', () => {
  const seed = core.createSeedTransactions();
  const before = JSON.stringify(seed);
  core.filterAndSort(seed, { search: 'salary', sort: 'amount-asc' });
  assert.strictEqual(JSON.stringify(seed), before);
});

console.log('\n== budget ==');
test('zero budget => percent 0, no NaN/Infinity', () => {
  const s = core.budgetStats(core.createSeedTransactions(), 0);
  assert.ok(Number.isFinite(s.percent) && s.percent === 0);
  assert.ok(Number.isFinite(s.over) && s.over === 0);
});
test('missing budget => safe too', () => {
  const s = core.budgetStats(core.createSeedTransactions(), null);
  assert.ok(Number.isFinite(s.percent) && s.percent === 0);
});
test('overspending yields > 100% percent + positive over', () => {
  const now = new Date();
  const key = core.currentMonthKey() + '-';
  const txs = [
    { type: 'expense', amount: 600, date: key + '01' },
    { type: 'expense', amount: 600, date: key + '02' },
    { type: 'expense', amount: 600, date: key + '03' },
    { type: 'expense', amount: 600, date: key + '04' }
  ];
  const s = core.budgetStats(txs, 2000);
  assert.strictEqual(s.spent, 2400);
  assert.ok(s.percent > 100, 'percent=' + s.percent);
  assert.strictEqual(s.over, 400);
});
test('only current-month expenses count', () => {
  const now = new Date();
  const y = now.getFullYear(), m = now.getMonth();
  const thisM = y + '-' + String(m + 1).padStart(2, '0') + '-05';
  const prev = y + '-' + String(m).padStart(2, '0') + '-05';
  const txs = [
    { type: 'expense', amount: 100, date: thisM },
    { type: 'expense', amount: 900, date: prev },
    { type: 'income', amount: 5000, date: thisM }
  ];
  assert.strictEqual(core.budgetStats(txs, 1000).spent, 100);
});

console.log(failures ? '\n' + failures + ' failure(s)' : '\nAll logic checks passed');
process.exit(failures ? 1 : 0);
