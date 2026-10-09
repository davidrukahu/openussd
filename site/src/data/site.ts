// Every fact on the site lives here. They mirror README.md, CHANGELOG.md and
// docs/telco-access.md: when one of those changes, change this file in the
// same pull request.

export const repo = 'https://github.com/davidrukahu/openussd';
export const blob = `${repo}/blob/main`;
export const tree = `${repo}/tree/main`;
export const codeberg = 'https://codeberg.org/davidrukahu/openussd';
export const issue = (n: number) => `${repo}/issues/${n}`;

export const title = 'OpenUSSD: reach any phone with a *123# menu';
export const description =
  'Free, open-source software that lets any organisation build a USSD menu, the *123# kind that works on every phone without internet, once for every network.';

export interface NavLink { href: string; label: string }

export const navLinks: NavLink[] = [
  { href: '/#how', label: 'How it works' },
  { href: '/#status', label: 'Status' },
  { href: '/#faq', label: 'FAQ' },
  { href: `${tree}/docs`, label: 'Docs' },
];

/** A real session against v0.1. The posts are demo data from a local test
 *  instance; every screen and budget figure is what the gateway produced. */
export interface Screen { caption: string; body: string; cost: string; encoding: 'GSM-7' | 'UCS-2' }

export const screens: Screen[] = [
  {
    caption: 'Dial the shortcode. The gateway routes it to the Fediverse tenant. This exact screen reached the Africa\'s Talking sandbox on 9 October 2026.',
    body: 'OpenUSSD Fediverse\n1. Public timeline\n2. About\n0. Quit',
    cost: '54 / 182',
    encoding: 'GSM-7',
  },
  {
    caption: 'Display names lose their emoji, so the list stays GSM-7 and more posts fit.',
    body: 'Latest posts\n1. Kiambu Farmers Coop (now)\n2. Shule ya Msingi (3m)\n3. Ward Office (9m)\n0. Quit',
    cost: '93 / 182',
    encoding: 'GSM-7',
  },
  {
    caption: 'A Swahili post ends in an emoji. The text stays GSM-7 and gets the full 182.',
    body: 'Shule ya Msingi (1/2)\nHabari za asubuhi! Mkutano wa\nwazazi utafanyika Jumamosi saa\nnne asubuhi katika ukumbi wa\nshule. Tafadhali fika mapema\n1. Next 0. Back',
    cost: '156 / 182',
    encoding: 'GSM-7',
  },
  {
    caption: 'Only the screen holding the emoji pays for UCS-2, where the budget is 70.',
    body: 'Shule ya Msingi (2/2)\n🙏\n2. Prev 0. Back',
    cost: '40 / 70',
    encoding: 'UCS-2',
  },
];

/** [text, link, link label] */
export type Ledger = [string, string | null, string | null];

export const working: Ledger[] = [
  ['The core that receives each call and keeps track of every conversation', `${blob}/docs/rfcs/0001-telco-adapter-interface.md`, 'design'],
  ["A connection to Africa's Talking, tested against their live test system", issue(7), '#7'],
  ['Conversations remembered between screens, until 3 minutes without a reply', null, null],
  ['Signed messages to the app that owns each code, so it knows they are genuine', `${blob}/docs/webhook-protocol.md`, 'protocol'],
  ['A Go toolkit for building menus that always fit the screen, in several languages', 'https://pkg.go.dev/github.com/davidrukahu/openussd/sdk/go', 'pkg.go.dev'],
  ['A demo that reads Mastodon posts on a basic phone', null, null],
];

export const notYet: Ledger[] = [
  ['SMS', null, null],
  ['A second network, such as MTN', null, null],
  ['A toolkit for TypeScript (JavaScript)', null, null],
  ['Posting to Mastodon from a phone, and linking a phone to an account', issue(9), '#9'],
  ['A permanent record of each session', null, null],
  ['An independent security review', null, null],
];

export const limitations: string[] = [
  'Someone who captures a message from OpenUSSD to an app could send it again within 5 minutes. Until this is fixed, apps should ignore repeats.',
  'The test phone that comes with OpenUSSD has no security. It is only for trying OpenUSSD on your own computer.',
  'If the phone company retries a step, the app can receive it twice.',
  'There are no ready-made downloads yet. You build it from the source code or run it with Docker.',
];

/** [name, link, description] */
export const related: [string, string, string][] = [
  ['Vumi and Junebug', 'https://github.com/praekeltfoundation/vumi', "Praekelt's Python messaging stack. Archived; last commits in 2020 and 2021."],
  ['vumi2', 'https://github.com/praekeltfoundation/vumi2', "Praekelt's actively developed Python 3 successor to Vumi, with USSD and SMPP transports."],
  ['RapidPro', 'https://github.com/rapidpro/rapidpro', 'A flow builder for SMS and messaging channels. USSD flows were removed in 2019.'],
  ['laravel-ussd', 'https://github.com/spesohq/laravel-ussd', 'A capable PHP framework that renders USSD responses for several aggregators. An application framework, not a gateway.'],
  ['RestComm USSD Gateway', 'https://github.com/RestComm/ussdgateway', 'An AGPL SS7/MAP gateway, inactive since 2018.'],
];

export const quickstart = `git clone https://github.com/davidrukahu/openussd
cd openussd
export FEDIVERSE_WEBHOOK_SECRET=$(openssl rand -hex 32)
docker compose up --build

# in a second terminal
go run ./cmd/ussdsim -shortcode '*384*1234#'`;

export interface FaqItem { q: string; a: string }

/** Answers are trusted HTML written here, so they can carry links. */
export const faqs: FaqItem[] = [
  {
    q: 'What is USSD?',
    a: 'The menu you get when you dial a code like *334# for M-Pesa. You reply with a number. It works on every phone, with no app and no internet.',
  },
  {
    q: 'Who is OpenUSSD for?',
    a: 'Clinics, schools, savings groups, co-ops, local government, and the developers who build for them. People who dial the code only see the menu.',
  },
  {
    q: 'Do I need an agreement with a phone company?',
    a: `Not to build and test. To go live, you need an agreement with Africa's Talking or a phone company. <a href="${blob}/docs/telco-access.md">Here is what is possible today</a>.`,
  },
  {
    q: 'Which networks does it support?',
    a: `Africa's Talking today, which reaches Kenya, Uganda, Nigeria, Rwanda, Tanzania and Malawi. MTN and others are planned.`,
  },
  {
    q: 'Is it ready to use with real people?',
    a: 'Not yet. It is an early version, and parts will change before version 1.0. See <a href="#limitations">known limitations</a>.',
  },
  {
    q: 'What does it cost?',
    a: `The software is free (<a href="${blob}/LICENSE">AGPL-3.0</a>). You pay only for hosting, and for each session Africa's Talking or a phone company charges.`,
  },
  {
    q: 'How is it funded?',
    a: `It is unfunded so far. <a href="${blob}/ROADMAP.md">The roadmap</a> shows what funding would pay for.`,
  },
];

export interface FooterColumn { title: string; links: { href: string; label: string }[] }

export const footerColumns: FooterColumn[] = [
  {
    title: 'Project',
    links: [
      { href: repo, label: 'GitHub' },
      { href: codeberg, label: 'Codeberg mirror' },
      { href: `${blob}/CHANGELOG.md`, label: 'Changelog' },
      { href: `${blob}/docs/architecture.md`, label: 'Architecture' },
      { href: `${blob}/docs/webhook-protocol.md`, label: 'Webhook protocol' },
    ],
  },
  {
    title: 'Maintainer',
    links: [{ href: 'https://github.com/davidrukahu', label: 'David W, Nairobi, Kenya' }],
  },
  {
    title: 'Licence',
    links: [
      { href: `${blob}/LICENSE`, label: 'AGPL-3.0-or-later code' },
      { href: 'https://creativecommons.org/licenses/by-sa/4.0/', label: 'CC BY-SA 4.0 docs' },
    ],
  },
];
