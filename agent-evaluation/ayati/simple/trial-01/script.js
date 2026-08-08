/* ============================================================
   Orbit 2026 — interactions
   ============================================================ */
(function () {
  "use strict";

  var root = document.documentElement;
  var THEME_KEY = "orbit-theme";

  /* ---------- Theme ---------- */
  var themeToggle = document.getElementById("theme-toggle");

  function applyTheme(theme) {
    root.setAttribute("data-theme", theme);
    if (themeToggle) {
      themeToggle.setAttribute("aria-pressed", theme === "dark" ? "true" : "false");
      themeToggle.setAttribute(
        "aria-label",
        theme === "dark" ? "Switch to light theme" : "Switch to dark theme"
      );
    }
  }

  // Restore saved theme, falling back to system preference.
  var savedTheme = null;
  try {
    savedTheme = localStorage.getItem(THEME_KEY);
  } catch (e) {
    savedTheme = null;
  }

  if (savedTheme === "light" || savedTheme === "dark") {
    applyTheme(savedTheme);
  } else {
    var prefersDark = window.matchMedia &&
      window.matchMedia("(prefers-color-scheme: dark)").matches;
    applyTheme(prefersDark ? "dark" : "light");
  }

  if (themeToggle) {
    themeToggle.addEventListener("click", function () {
      var next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
      applyTheme(next);
      try {
        localStorage.setItem(THEME_KEY, next);
      } catch (e) {
        /* storage unavailable — theme still applies for this session */
      }
    });
  }

  /* ---------- Mobile navigation ---------- */
  var navToggle = document.getElementById("nav-toggle");
  var navLinks = document.getElementById("nav-links");

  function setNav(open) {
    if (!navToggle || !navLinks) return;
    navToggle.setAttribute("aria-expanded", open ? "true" : "false");
    navLinks.classList.toggle("open", open);
  }

  if (navToggle && navLinks) {
    navToggle.addEventListener("click", function () {
      setNav(navToggle.getAttribute("aria-expanded") !== "true");
    });

    // Close the menu when a link is chosen.
    navLinks.addEventListener("click", function (event) {
      if (event.target.closest("a")) {
        setNav(false);
      }
    });

    // Close on Escape.
    document.addEventListener("keydown", function (event) {
      if (event.key === "Escape" && navToggle.getAttribute("aria-expanded") === "true") {
        setNav(false);
        navToggle.focus();
      }
    });
  }

  /* ---------- Newsletter form ---------- */
  var form = document.getElementById("newsletter-form");
  var emailInput = document.getElementById("email");
  var formStatus = document.getElementById("form-status");

  var EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

  function setStatus(message, state) {
    if (!formStatus) return;
    formStatus.textContent = message;
    if (state) {
      formStatus.setAttribute("data-state", state);
    } else {
      formStatus.removeAttribute("data-state");
    }
  }

  if (form && emailInput && formStatus) {
    form.addEventListener("submit", function (event) {
      event.preventDefault();

      var value = emailInput.value.trim();

      if (!value) {
        emailInput.setAttribute("aria-invalid", "true");
        setStatus("Please enter your email address.", "error");
        emailInput.focus();
        return;
      }

      if (!EMAIL_RE.test(value)) {
        emailInput.setAttribute("aria-invalid", "true");
        setStatus("That doesn't look like a valid email address.", "error");
        emailInput.focus();
        return;
      }

      emailInput.removeAttribute("aria-invalid");
      setStatus("Thanks! You're on the list — see you at Orbit 2026.", "success");
      form.reset();
    });

    // Clear the error state as the user corrects their input.
    emailInput.addEventListener("input", function () {
      if (emailInput.getAttribute("aria-invalid") === "true") {
        emailInput.removeAttribute("aria-invalid");
        setStatus("", null);
      }
    });
  }

  /* ---------- Animated stat counters ---------- */
  var reduceMotion = window.matchMedia &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  var statNumbers = Array.prototype.slice.call(
    document.querySelectorAll(".stat-number[data-count]")
  );

  function animateCount(el) {
    var target = parseInt(el.getAttribute("data-count"), 10);
    var duration = 1400;
    var start = null;

    function format(n) {
      return n.toLocaleString("en-US");
    }

    function step(timestamp) {
      if (!start) start = timestamp;
      var progress = Math.min((timestamp - start) / duration, 1);
      // easeOutCubic
      var eased = 1 - Math.pow(1 - progress, 3);
      el.textContent = format(Math.round(target * eased));
      if (progress < 1) {
        window.requestAnimationFrame(step);
      }
    }

    window.requestAnimationFrame(step);
  }

  if (statNumbers.length && !reduceMotion) {
    if ("IntersectionObserver" in window) {
      var observer = new IntersectionObserver(
        function (entries, obs) {
          entries.forEach(function (entry) {
            if (entry.isIntersecting) {
              animateCount(entry.target);
              obs.unobserve(entry.target);
            }
          });
        },
        { threshold: 0.4 }
      );
      statNumbers.forEach(function (el) {
        observer.observe(el);
      });
    } else {
      statNumbers.forEach(animateCount);
    }
  } else {
    // Reduced motion or no elements: show final values immediately.
    statNumbers.forEach(function (el) {
      el.textContent = parseInt(el.getAttribute("data-count"), 10).toLocaleString("en-US");
    });
  }
})();
