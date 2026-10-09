// Behaviour modules, one per section. Every module takes the reduced-motion
// flag and degrades to a static layout when it is set.
import { init as nav } from './nav.js';
import { init as mobileNav } from './mobile-nav.js';
import { init as hero } from './hero.js';
import { init as whycards } from './why-cards.js';
import { init as delegation } from './delegation.js';
import { init as howitworks } from './how-it-works.js';
import { init as faq } from './faq.js';
import { init as externalLinks } from './external-links.js';

const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
nav(reduced);
mobileNav(reduced);
hero(reduced);
whycards(reduced);
delegation(reduced);
howitworks(reduced);
faq(reduced);
externalLinks();
