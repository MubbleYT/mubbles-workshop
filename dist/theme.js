(() => {
  'use strict';
  const key = 'mubbles-workshop-theme';
  const preference = window.matchMedia('(prefers-color-scheme: dark)');
  let saved;
  try { saved = localStorage.getItem(key); } catch {}
  let chosen = saved === 'light' || saved === 'dark' ? saved : null;

  function apply(theme) {
    document.documentElement.dataset.theme = theme;
    const toggle = document.getElementById('theme-toggle');
    if (toggle) toggle.setAttribute('aria-checked', String(theme === 'dark'));
  }

  apply(chosen || (preference.matches ? 'dark' : 'light'));
  document.addEventListener('DOMContentLoaded', () => {
    apply(document.documentElement.dataset.theme);
    document.getElementById('theme-toggle').addEventListener('click', () => {
      chosen = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
      apply(chosen);
      try { localStorage.setItem(key, chosen); } catch {}
    });
  });
  preference.addEventListener('change', event => {
    if (!chosen) apply(event.matches ? 'dark' : 'light');
  });
  window.addEventListener('storage', event => {
    if (event.key !== key) return;
    chosen = event.newValue === 'light' || event.newValue === 'dark' ? event.newValue : null;
    apply(chosen || (preference.matches ? 'dark' : 'light'));
  });
})();
