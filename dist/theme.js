(() => {
  'use strict';
  const key = 'mubbles-workshop-theme';
  const preference = window.matchMedia('(prefers-color-scheme: dark)');
  let saved;
  try { saved = localStorage.getItem(key); } catch {}
  let chosen = saved === 'light' || saved === 'dark' ? saved : null;
  let fadeTimer;

  function apply(theme) {
    document.documentElement.dataset.theme = theme;
    const toggle = document.getElementById('theme-toggle');
    if (toggle) toggle.setAttribute('aria-checked', String(theme === 'dark'));
  }

  function fadeTo(theme) {
    document.documentElement.classList.add('theme-changing');
    // Establish the old palette with transitions enabled before changing it.
    void document.body.offsetWidth;
    apply(theme);
    clearTimeout(fadeTimer);
    fadeTimer = setTimeout(() => document.documentElement.classList.remove('theme-changing'), 700);
  }

  apply(chosen || (preference.matches ? 'dark' : 'light'));
  document.addEventListener('DOMContentLoaded', () => {
    apply(document.documentElement.dataset.theme);
    document.getElementById('theme-toggle').addEventListener('click', () => {
      chosen = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
      fadeTo(chosen);
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
