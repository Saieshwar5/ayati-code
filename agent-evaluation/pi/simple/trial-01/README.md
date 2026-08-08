# Orbit 2026 — Conference Website

A polished, responsive single-page website for a fictional technology conference,
built with **plain HTML, CSS, and vanilla JavaScript**. No frameworks, no build
step, no CDNs, and no network requests — it runs from any static file server.

## Features

- **Sticky navigation bar** with a mobile hamburger button (`#nav-toggle`) that
  toggles `aria-expanded`, shows/hides the menu, closes on link click, Escape,
  outside click, and viewport growth.
- **Hero section** with CSS-only orbital decoration.
- **Event statistics** that count up when scrolled into view (respects
  `prefers-reduced-motion`).
- **Speaker cards** — 8 speakers rendered with inline SVG/custom initials
  avatars (no external images).
- **Schedule** — an accessible ARIA tabs pattern for the three conference days
  (arrow-key navigation included).
- **Ticket tiers** — Explorer, Voyager (featured), and Commander cards.
- **Newsletter form** (`#newsletter-form`, `#email`, `#form-status`):
  invalid submissions show an error, valid ones show a success message —
  never a page reload.
- **Light/dark theme toggle** (`#theme-toggle`) persisted in `localStorage`
  under the key `orbit-theme`. The saved theme is applied before first paint to
  prevent flashing; the default tracks the OS preference.
- **Accessibility**: skip link, semantic landmarks, visible focus rings,
  ARIA states on interactive elements, sufficient color contrast, and a
  `prefers-reduced-motion` fallback that disables animations/transitions.
- **Responsive**: intentionally designed and verified at 390px (mobile
  single-column) and 1440px (multi-column desktop grids).

## Project structure

```
.
├── index.html      # Single-page markup (all sections)
├── css/styles.css  # Design tokens, themes, layouts, responsive + motion rules
├── js/main.js      # Nav, theme, tabs, form validation, stats
└── README.md
```

## How to run

Any static HTTP server works. From this directory:

**Python 3**

```sh
python3 -m http.server 8080
```

**Python 2**

```sh
python -m SimpleHTTPServer 8080
```

> Any static file server works (e.g. `npx http-server .`), and opening
> `index.html` directly via `file://` is fine for inspection too — the site
> makes no network requests.

Then open <http://localhost:8080>.

> Using `file://` works for most interactions, but a static server is
> recommended and is what the site targets.

## How to inspect

### The build (quick verification)

```sh
node --check js/main.js          # syntax-check the JavaScript
grep -n 'id="' index.html        # confirm required IDs: nav-toggle, theme-toggle,
                                 # newsletter-form, email, form-status
```

### Theme behavior

1. Click the moon/sun button — the `data-theme` attribute on `<html>` flips and
   `localStorage["orbit-theme"]` is set to `"light"` or `"dark"`.
2. Reload — the choice is restored. Clear storage (DevTools → Application →
   Local Storage) to fall back to `prefers-color-scheme`.

### Responsive layouts

Open DevTools device toolbar (Ctrl/Cmd+Shift+M) and check:

- **390px**: hamburger menu with working `aria-expanded`; stacked sections;
  stats in a 2×2 grid; single-column tickets and speakers.
- **1440px**: sticky header links inline; 4-column speaker grid, 3-column
  tickets, side-by-side about / newsletter layouts.

### Accessibility spot-checks

- Tab through the page — focus rings stay visible everywhere (including the
  skip link, which appears on first Tab).
- Emulate `prefers-reduced-motion: reduce` in DevTools → Rendering — scroll
  behavior becomes instant and stats skip the count-up animation.
- Run Lighthouse (Audits) for an automated check of contrast, landmarks, and
  forms.

## Notes

- Copy, people, and events are fictional.
- The brand mark and social icons are inline SVG so the page never fetches
  remote assets.
