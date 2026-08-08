# Orbit 2026

A polished, responsive single-page website for a fictional technology conference,
built with **only HTML, CSS, and vanilla JavaScript** — no external packages, CDNs,
remote images, or network resources.

## Files

| File          | Purpose                                              |
| ------------- | ---------------------------------------------------- |
| `index.html`  | Entry point — all page content and structure         |
| `styles.css`  | Styling, theming, responsive layout, accessibility   |
| `script.js`   | Theme, mobile nav, newsletter form, stat counters    |
| `README.md`   | This file                                            |

## Running the site

The site is fully static and works with any basic static HTTP server. From this
directory:

```bash
# Python 3
python3 -m http.server 8000
```

```bash
# Node.js
npx serve .
```

```bash
# PHP
php -S localhost:8000
```

Then open <http://localhost:8000> in your browser. `index.html` is the entry point.

## Inspecting the site

- **Responsive layout** — resize the window or use DevTools device toolbar. The
  layout is tuned for 390px (mobile) and 1440px (desktop) widths.
- **Mobile navigation** — below 640px the hamburger button (`#nav-toggle`) appears.
  It toggles the `aria-expanded` attribute and shows/hides the nav links.
- **Theme toggle** — the `#theme-toggle` button switches between light and dark
  themes. The choice is persisted in `localStorage` under the key `orbit-theme`.
  On first visit it follows the OS `prefers-color-scheme`.
- **Newsletter form** — the `#newsletter-form` validates the `#email` input without
  reloading the page. Invalid submissions show an error in `#form-status`; valid
  submissions show a success message.

## Accessibility

- Semantic HTML (`header`, `nav`, `main`, `section`, `article`, `footer`, `time`).
- Visible `:focus-visible` outlines and a skip-to-content link.
- `aria-expanded` / `aria-controls` on the nav toggle, `aria-pressed` on the theme
  toggle, and `aria-live` on the form status.
- Sufficient color contrast in both themes.
- `prefers-reduced-motion` is respected: smooth scrolling, transitions, and the
  animated stat counters are disabled, and final stat values are shown immediately.
