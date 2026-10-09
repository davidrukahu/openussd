// @ts-check
import { defineConfig } from 'astro/config';

// Static output only. The page carries no analytics and loads nothing from a
// third-party host: fonts, images and scripts are all served from the site.
export default defineConfig({
  output: 'static',
  site: 'https://openussd.org',
  devToolbar: { enabled: false },
});
