/* ============================================================
   Ledgerly — theme control (light / dark)
   Persists under "ledgerly-theme".
   ============================================================ */

const Theme = (() => {
  const root = document.documentElement;
  const toggleBtn = document.getElementById('theme-toggle');

  function apply(theme) {
    const resolved = theme === 'dark' ? 'dark' : 'light';
    root.setAttribute('data-theme', resolved);
    if (toggleBtn) {
      toggleBtn.setAttribute('aria-pressed', String(resolved === 'dark'));
      toggleBtn.setAttribute('aria-label', resolved === 'dark'
        ? 'Switch to light theme'
        : 'Switch to dark theme');
    }
  }

  function init() {
    const saved = Storage.getTheme();
    apply(saved === 'dark' ? 'dark' : 'light');

    if (toggleBtn) {
      toggleBtn.addEventListener('click', () => {
        const next = root.getAttribute('data-theme') === 'dark' ? 'light' : 'dark';
        apply(next);
        Storage.saveTheme(next);
      });
    }
  }

  return { init };
})();
