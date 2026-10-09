/**
 * Hero: the panel starts where Framer puts it (1240x619, 24px radius, inset in
 * the container) and expands to full bleed as the section scrolls away, the
 * radius flattening with it. The image inside also scales, which is what the
 * Framer original does on its own (~0.00023 per scrolled pixel, measured off
 * the live site).
 *
 * The resting values live in CSS, so with JS off or reduced motion on, the
 * hero simply stays in its Framer position.
 */
export function init(reduced) {
  const hero = document.getElementById('hero');
  const panel = document.getElementById('hero-media');
  const img = document.querySelector('.hero__img');
  if (!hero || !panel || !img || reduced) return;

  const TRAVEL = 500;      // px of scroll over which the panel opens out
  const RADIUS = 24;
  const IMG_RATE = 0.00023;
  const IMG_MAX = 1.22;

  const lerp = (a, b, t) => a + (b - a) * t;
  const smooth = (t) => t * t * (3 - 2 * t);

  const update = () => {
    const y = Math.max(0, window.scrollY);
    const p = smooth(Math.min(1, y / TRAVEL));

    const rest = hero.getBoundingClientRect();
    const vw = document.documentElement.clientWidth;
    // Read the resting geometry rather than assuming it: the container gutter
    // and the panel's own height both change at the phone breakpoint.
    const cs = getComputedStyle(hero);
    const restTop = parseFloat(cs.getPropertyValue('--hero-top')) || 20;
    const restH = parseFloat(cs.getPropertyValue('--hero-rest-h')) || 619;
    const restW = Math.min(1240, rest.width);

    const w = lerp(restW, vw, p);
    const h = lerp(restH, hero.offsetHeight, p);
    const dy = lerp(0, -restTop, p);

    panel.classList.toggle('is-expanding', p > 0);
    panel.style.setProperty('--w', `${w}px`);
    panel.style.setProperty('--h', `${h}px`);
    panel.style.setProperty('--y', `${dy}px`);
    panel.style.setProperty('--radius', `${lerp(RADIUS, 0, p)}px`);
    img.style.setProperty('--img-scale', Math.min(IMG_MAX, 1 + IMG_RATE * y).toFixed(4));
  };

  let ticking = false;
  const onScroll = () => {
    if (ticking) return;
    ticking = true;
    requestAnimationFrame(() => { update(); ticking = false; });
  };

  window.addEventListener('scroll', onScroll, { passive: true });
  window.addEventListener('resize', onScroll, { passive: true });
  update();
}
