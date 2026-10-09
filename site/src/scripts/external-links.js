// Links that leave the site (GitHub, Codeberg, sources) open in a new tab,
// so a reader does not lose their place on the page. Links within the site
// and to sections of this page stay in the same tab.
export function init() {
  for (const link of document.querySelectorAll('a[href]')) {
    const url = new URL(link.href, location.href);
    if (!url.protocol.startsWith('http') || url.host === location.host) continue;
    link.target = '_blank';
    link.rel = 'noopener noreferrer';
  }
}
