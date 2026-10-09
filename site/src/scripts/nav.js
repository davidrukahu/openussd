/** From the Parley template (MIT). */
export function init(reduced) {
  // ============ NAV: hide on scroll down, show on scroll up; bg-aware ============
  const navBar = document.querySelector(".nav-bar");
  const heroSection = document.getElementById("hero");
  if (navBar) {
    const REVEAL_AT_TOP = 80;
    const DELTA = 6;
    let lastY = window.scrollY;
    let lastDir = 0;

    const onNav = () => {
      const y = window.scrollY;
      const dy = y - lastY;

      if (y <= REVEAL_AT_TOP) {
        navBar.classList.remove("is-hidden");
        lastDir = 0;
      } else if (Math.abs(dy) >= DELTA) {
        const dir = dy > 0 ? 1 : -1;
        if (dir !== lastDir) {
          if (dir === 1) navBar.classList.add("is-hidden");
          else navBar.classList.remove("is-hidden");
          lastDir = dir;
        }
      }

      if (heroSection) {
        const navH = navBar.offsetHeight || 60;
        const heroBottomInViewport = heroSection.getBoundingClientRect().bottom;
        navBar.classList.toggle("is-over-hero", heroBottomInViewport > navH);
      }

      lastY = y;
    };
    window.addEventListener("scroll", onNav, { passive: true });
    onNav();
  }
}
