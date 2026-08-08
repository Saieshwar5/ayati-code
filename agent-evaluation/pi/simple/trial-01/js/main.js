/* ==========================================================================
   Orbit 2026 — interactions
   1. Mobile navigation toggle
   2. Light/dark theme with localStorage persistence
   3. Schedule day tabs (ARIA tabs pattern)
   4. Newsletter form validation
   5. Count-up statistics (respects prefers-reduced-motion)
   ========================================================================== */

(function () {
  "use strict";

  var doc = document;
  var root = doc.documentElement;

  function prefersReducedMotion() {
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }

  /* ---------------------------------- 1. Nav ------------------------------ */

  var navToggle = doc.getElementById("nav-toggle");
  var siteHeader = doc.getElementById("site-header");
  var primaryMenu = doc.getElementById("primary-menu");

  function closeNav() {
    if (!siteHeader) return;
    siteHeader.classList.remove("nav-open");
    if (navToggle) {
      navToggle.setAttribute("aria-expanded", "false");
      navToggle.setAttribute("aria-label", "Open navigation menu");
    }
  }

  function toggleNav() {
    var isOpen = siteHeader.classList.toggle("nav-open");
    if (navToggle) {
      navToggle.setAttribute("aria-expanded", isOpen ? "true" : "false");
      navToggle.setAttribute("aria-label", isOpen ? "Close navigation menu" : "Open navigation menu");
    }
  }

  if (navToggle && primaryMenu) {
    navToggle.setAttribute("aria-label", "Open navigation menu");
    navToggle.addEventListener("click", toggleNav);

    // Close when a nav link is chosen (mobile).
    primaryMenu.addEventListener("click", function (event) {
      if (event.target.closest("a")) closeNav();
    });

    // Close on Escape.
    doc.addEventListener("keydown", function (event) {
      if (event.key === "Escape" && siteHeader.classList.contains("nav-open")) {
        closeNav();
        navToggle.focus();
      }
    });

    // Close when clicking outside the header.
    doc.addEventListener("click", function (event) {
      if (
        siteHeader.classList.contains("nav-open") &&
        !event.target.closest(".site-header")
      ) {
        closeNav();
      }
    });

    // Keep state consistent when the viewport grows past the mobile breakpoint.
    window.addEventListener("resize", function () {
      if (window.innerWidth > 860 && siteHeader.classList.contains("nav-open")) {
        closeNav();
      }
    });
  }

  /* ---------------------------------- 2. Theme --------------------------- */

  var themeToggle = doc.getElementById("theme-toggle");
  var STORAGE_KEY = "orbit-theme";
  // Sun/moon icons live in the markup; CSS swaps them via [data-theme].

  function currentTheme() {
    return root.getAttribute("data-theme") === "dark" ? "dark" : "light";
  }

  function applyTheme(theme) {
    root.setAttribute("data-theme", theme);
    var isDark = theme === "dark";
    if (themeToggle) themeToggle.setAttribute("aria-pressed", String(isDark));
    if (themeToggle) {
      themeToggle.setAttribute(
        "aria-label",
        isDark ? "Switch to light theme" : "Switch to dark theme"
      );
    }
    try {
      localStorage.setItem(STORAGE_KEY, theme);
    } catch (e) {
      /* storage unavailable — theme still applies for this session */
    }
  }

  if (themeToggle) {
    applyTheme(currentTheme()); // sync label/state with initial inline script
    themeToggle.addEventListener("click", function () {
      applyTheme(currentTheme() === "dark" ? "light" : "dark");
    });
  }

  /* ---------------------------------- 3. Schedule tabs ------------------- */

  var tabs = Array.prototype.slice.call(doc.querySelectorAll('[role="tab"]'));
  var panels = Array.prototype.slice.call(doc.querySelectorAll('[role="tabpanel"]'));

  function selectTab(tab) {
    if (!tab || tab.getAttribute("aria-selected") === "true") return;

    tabs.forEach(function (t) {
      t.setAttribute("aria-selected", String(t === tab));
      t.setAttribute("tabindex", t === tab ? "0" : "-1");
    });

    panels.forEach(function (panel) {
      var show = panel.getAttribute("aria-labelledby") === tab.id;
      panel.hidden = !show;
      panel.classList.toggle("is-active", show);
    });

    tab.focus({ preventScroll: true });
  }

  if (tabs.length) {
    tabs.forEach(function (tab) {
      tab.addEventListener("click", function () {
        selectTab(tab);
      });

      tab.addEventListener("keydown", function (event) {
        var index = tabs.indexOf(tab);
        var next = null;

        if (event.key === "ArrowRight") next = tabs[(index + 1) % tabs.length];
        else if (event.key === "ArrowLeft") next = tabs[(index - 1 + tabs.length) % tabs.length];
        else if (event.key === "Home") next = tabs[0];
        else if (event.key === "End") next = tabs[tabs.length - 1];

        if (next) {
          event.preventDefault();
          selectTab(next);
        }
      });
    });
  }

  /* ---------------------------------- 4. Newsletter ---------------------- */

  var form = doc.getElementById("newsletter-form");
  var emailInput = doc.getElementById("email");
  var formStatus = doc.getElementById("form-status");

  // Simple but practical email check: local@domain.tld
  var EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/;

  function setStatus(message, state) {
    if (!formStatus) return;
    formStatus.textContent = message;
    formStatus.setAttribute("data-state", state || "");
    formStatus.setAttribute("aria-hidden", message ? "false" : "true");
  }

  if (form && emailInput && formStatus) {
    // Remove the default empty aria-hidden once scripts run.
    formStatus.setAttribute("aria-hidden", "true");

    emailInput.addEventListener("input", function () {
      if (emailInput.getAttribute("aria-invalid") === "true") {
        emailInput.removeAttribute("aria-invalid");
        var clean = emailInput.value.trim();
        if (clean && EMAIL_RE.test(clean)) {
          setStatus("", "");
        }
      }
    });

    form.addEventListener("submit", function (event) {
      event.preventDefault(); // never reload

      var value = emailInput.value.trim();

      if (!value) {
        emailInput.setAttribute("aria-invalid", "true");
        emailInput.focus();
        setStatus("Please enter your email address.", "error");
        return;
      }

      if (!EMAIL_RE.test(value)) {
        emailInput.setAttribute("aria-invalid", "true");
        emailInput.focus();
        setStatus("That doesn’t look like a valid email — try you@example.com.", "error");
        return;
      }

      // Valid: success without a page reload.
      emailInput.removeAttribute("aria-invalid");
      form.reset();
      setStatus("You’re on the list! Watch your inbox for launch updates.", "success");
      emailInput.focus();
    });
  }

  /* ---------------------------------- 5. Stats --------------------------- */

  var stats = doc.getElementById("stats");

  function animateCount(el) {
    var target = parseInt(el.getAttribute("data-count"), 10);
    if (Number.isNaN(target)) return;

    var duration = 1200;
    var start = null;

    function tick(timestamp) {
      if (start === null) start = timestamp;
      var progress = Math.min((timestamp - start) / duration, 1);
      // ease-out cubic
      var eased = 1 - Math.pow(1 - progress, 3);
      el.textContent = Math.round(target * eased).toLocaleString("en-US");
      if (progress < 1) window.requestAnimationFrame(tick);
      else el.textContent = target.toLocaleString("en-US");
    }

    window.requestAnimationFrame(tick);
  }

  if (stats && "IntersectionObserver" in window) {
    var statNumbers = Array.prototype.slice.call(stats.querySelectorAll("[data-count]"));

    if (prefersReducedMotion()) {
      // Respect reduced motion: show final values immediately.
      statNumbers.forEach(function (el) {
        el.textContent = parseInt(el.getAttribute("data-count"), 10).toLocaleString("en-US");
      });
    } else {
      var observer = new IntersectionObserver(
        function (entries) {
          entries.forEach(function (entry) {
            if (entry.isIntersecting) {
              animateCount(entry.target);
              observer.unobserve(entry.target);
            }
          });
        },
        { threshold: 0.4 }
      );
      statNumbers.forEach(function (el) {
        observer.observe(el);
      });
    }
  } else if (stats) {
    // No observer support: just print the final numbers.
    stats.querySelectorAll("[data-count]").forEach(function (el) {
      el.textContent = parseInt(el.getAttribute("data-count"), 10).toLocaleString("en-US");
    });
  }
})();
