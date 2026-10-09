// @ts-check
import { defineConfig } from 'astro/config';

// Static output only: the page has to work with JavaScript turned off and
// carries no analytics.
export default defineConfig({
  site: 'https://openussd.org',
});
