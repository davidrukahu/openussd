// Every fact on the site lives here. They mirror README.md, CHANGELOG.md and
// docs/telco-access.md: when one of those changes, change this file in the
// same pull request.

export const repo = 'https://github.com/davidrukahu/openussd';
export const blob = `${repo}/blob/main`;
export const tree = `${repo}/tree/main`;
export const codeberg = 'https://codeberg.org/davidrukahu/openussd';
export const issue = (n: number) => `${repo}/issues/${n}`;

export const title = 'OpenUSSD: an open gateway and SDK for the feature-phone web';
export const description =
  'OpenUSSD is an open-source, self-hostable USSD gateway and SDK. v0.1 runs end to end: dial a shortcode from your terminal and page through a Mastodon timeline, with no telco account.';

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
  ['Canonical session events and the adapter interface', `${blob}/docs/rfcs/0001-telco-adapter-interface.md`, 'RFC-0001'],
  ["Africa's Talking adapter, contract tests including captures from the live sandbox", issue(7), '#7'],
  ['Session store: in-memory and Redis, 180 s idle expiry', null, null],
  ['Tenant routing with HMAC-signed webhooks', `${blob}/docs/webhook-protocol.md`, 'protocol'],
  ['Go SDK: typed screens, state, i18n, encoding-aware screen budget', 'https://pkg.go.dev/github.com/davidrukahu/openussd/sdk/go', 'pkg.go.dev'],
  ['Mastodon public timeline over USSD, paginated', null, null],
];

export const notYet: Ledger[] = [
  ['SMS', null, null],
  ['A second real network', null, null],
  ['TypeScript SDK', null, null],
  ['Fediverse write paths and identity binding', issue(9), '#9'],
  ['Postgres audit store', null, null],
  ['Independent security audit', null, null],
];

export const limitations: string[] = [
  "Webhooks carry no replay nonce. The signature's 5-minute window bounds replay but does not prevent it, so tenants should deduplicate.",
  'The simulator adapter authenticates nothing. It is the demo; never expose it.',
  'Two callbacks for one session are not yet serialised; a retried callback can reach the tenant twice.',
  'No prebuilt binaries. Build from source or use Docker Compose.',
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
    q: 'Do I need a telco account?',
    a: `No, not for development. The terminal simulator and the free Africa's Talking sandbox need none. Production needs an agreement with an aggregator or an operator; <a href="${blob}/docs/telco-access.md">getting USSD access in practice</a> sets out what is obtainable today.`,
  },
  {
    q: 'Which networks does it support?',
    a: `Africa's Talking today, which reaches networks in Kenya, Uganda, Nigeria, Rwanda, Tanzania and Malawi. MTN and SMPP are planned. See <a href="${blob}/docs/telco-access.md">docs/telco-access.md</a>.`,
  },
  {
    q: 'Is it production ready?',
    a: 'No. v0.1 is a working spike, and interfaces will change before 1.0. See <a href="#limitations">Known limitations</a>.',
  },
  {
    q: 'What does it cost?',
    a: `Nothing. It is free software under <a href="${blob}/LICENSE">AGPL-3.0-or-later</a>. Running it costs whatever your hosting and your aggregator charge per session.`,
  },
  {
    q: 'How is it funded?',
    a: 'It is unfunded. An application to the NLnet NGI Zero Commons Fund is in second-round review.',
  },
  {
    q: 'Was AI used to build it?',
    a: `Yes, and <a href="${repo}#how-this-project-uses-ai-tools">the README says how</a>. The maintainer designed the architecture, the protocol and the RFCs, and directs and reviews every change. Much of the v0.1 Go implementation and this website were written with Claude Code. Grant-funded code will be written by people. AI tools may help with review, research and documentation, and any such use is disclosed.`,
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
