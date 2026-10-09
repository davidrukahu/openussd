/**
 * Mobile navigation. Mirrors the Framer component's Phone / Phone Open
 * variants: the menu expands with spring-physics 600 40 1, the separator
 * dots hide while open, and the panel scrolls if it outgrows the viewport.
 */
export function init(reduced) {
  const bar = document.querySelector('[data-nav]');
  const toggle = document.querySelector('[data-nav-toggle]');
  const menu = document.querySelector('[data-nav-menu]');
  if (!bar || !toggle || !menu) return;

  const setHeight = () => {
    // Animate to the menu's natural height so the spring has a target.
    // Measured from the last child's offset so the panel's bottom padding is
    // included; scrollHeight alone clips it in some engines.
    const last = menu.lastElementChild;
    const pad = parseFloat(getComputedStyle(menu).paddingBottom) || 0;
    const h = last ? last.offsetTop + last.offsetHeight + pad : menu.scrollHeight;
    menu.style.setProperty('--menu-h', Math.ceil(h) + 'px');
  };

  const close = () => {
    bar.classList.remove('is-open');
    toggle.setAttribute('aria-expanded', 'false');
    toggle.setAttribute('aria-label', 'Open menu');
  };

  toggle.addEventListener('click', () => {
    const open = bar.classList.toggle('is-open');
    bar.classList.toggle('nav-bar--open', open);
    if (open) requestAnimationFrame(() => requestAnimationFrame(setHeight));
    toggle.setAttribute('aria-expanded', String(open));
    toggle.setAttribute('aria-label', open ? 'Close menu' : 'Open menu');
  });

  // Any navigation or an Escape press closes it.
  menu.addEventListener('click', (e) => {
    if (e.target.closest('a')) close();
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && bar.classList.contains('is-open')) { close(); toggle.focus(); }
  });

  // The desktop layout has no panel, so drop the open state when we cross back.
  const wide = window.matchMedia('(min-width: 811px)');
  wide.addEventListener('change', (e) => { if (e.matches) close(); });

  window.addEventListener('resize', () => {
    if (bar.classList.contains('is-open')) setHeight();
  }, { passive: true });

  if (reduced) menu.style.setProperty('--menu-duration', '0.01ms');
}
