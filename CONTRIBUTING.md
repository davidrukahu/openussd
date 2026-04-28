# Contributing to OpenUSSD

Thank you for your interest in OpenUSSD. The project is in its planning phase; design feedback right now is at least as valuable as code.

## How you can help today

- **Open an issue** to share use cases, feature-phone constraints we should know about, or experience integrating with a specific MNO.
- **Comment on a [design RFC](docs/rfcs/)** — we are still shaping the protocol, the SDK API, and the telco adapter interface.
- **Pilot interest** — if you maintain a service that wants USSD reach, tell us so we can shape the SDK around real apps.

## How you can help once code lands

1. Find an issue tagged [`good first issue`](https://github.com/davidrukahu/openussd/labels/good%20first%20issue) or [`help wanted`](https://github.com/davidrukahu/openussd/labels/help%20wanted), or open one to discuss your idea before writing a large patch.
2. Fork, branch from `main`, and keep changes focused — one logical change per PR.
3. Run the language-appropriate formatter, linter, and tests before pushing (commands will be documented per-package once each component lands).
4. Open a PR using the template. Reference the issue it addresses.
5. A maintainer reviews. Expect substantive feedback on protocol-touching changes; we would rather discuss a design twice than break compatibility later.

## Development setup

Per-component setup will be documented in each subdirectory's README as it lands:

- `gateway/` — Go service, Postgres, Redis (TBD)
- `sdk/go/` — Go module
- `sdk/typescript/` — pnpm workspace
- `adapters/fediverse/` — Go (likely shares the gateway's HTTP plumbing)

## Coding standards

- **Go:** `gofmt` + `golangci-lint`. Public APIs get doc comments. Prefer `errors.Is`/`errors.As` over string comparison.
- **TypeScript:** strict mode on. ESLint + Prettier. No `any` without a comment justifying it.
- **Commits:** [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:`). Subject ≤72 chars. Body explains the *why*.
- **Tests:** new behaviour ships with tests. Telco-adapter changes need contract tests against recorded fixtures.

## Reporting security issues

Please **do not** open public issues for security vulnerabilities. Email the maintainer at the address listed in [`funding.json`](funding.json) with a description and reproduction steps. We aim to acknowledge within 72 hours.

## Licensing of contributions

By submitting a contribution, you agree that your work is licensed under the project's [LICENSE](LICENSE) (AGPL-3.0-or-later) and that you have the right to license it. We do not require a separate CLA. For substantial contributions a sign-off (`git commit -s`) following [Developer Certificate of Origin](https://developercertificate.org/) is appreciated but not yet enforced.

If individual SDK packages are split into their own repositories under Apache-2.0, we will reach out to active contributors before re-licensing those specific subtrees.

## Code of conduct

Participation is governed by our [Code of Conduct](CODE_OF_CONDUCT.md). Be respectful; assume good faith; accept that English is many contributors' second language.

## Communication

- **GitHub Issues / Discussions** — primary channel.
- A real-time chat (Matrix or similar) will be opened once the project has more than a handful of regular contributors.

Thanks for considering OpenUSSD.
