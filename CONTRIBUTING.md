# Contributing to OpenUSSD

Thank you for your interest in OpenUSSD. The project is in its planning phase; design feedback right now is at least as valuable as code.

## How you can help today

- **Open an issue** to share use cases, feature-phone constraints we should know about, or experience integrating with a specific MNO.
- **Comment on a [design RFC](docs/rfcs/)** — we are still shaping the protocol, the SDK API, and the telco adapter interface.
- **Pilot interest** — if you maintain a service that wants USSD reach, tell us so we can shape the SDK around real apps.

## How you can help once code lands

1. Find an issue tagged [`good first issue`](https://github.com/davidrukahu/openussd/labels/good%20first%20issue) or [`help wanted`](https://github.com/davidrukahu/openussd/labels/help%20wanted), or open one to discuss your idea before writing a large patch.
2. Fork, branch from `main`, and keep changes focused — one logical change per PR.
3. Run `make check` before pushing. Telco-adapter changes also need fixtures under `gateway/testdata/fixtures/<mno>/`, with their provenance recorded — a hand-written fixture proves the adapter is self-consistent, a captured one proves it matches the network.
4. Open a PR using the template. Reference the issue it addresses.
5. A maintainer reviews. Expect substantive feedback on protocol-touching changes; we would rather discuss a design twice than break compatibility later.

## Development setup

Go 1.24 or newer, and Docker if you want the compose demo.

```bash
git clone https://github.com/davidrukahu/openussd.git
cd openussd
make check      # formatting, vet, tests with the race detector
make build      # binaries into bin/
```

To run the whole thing and dial it:

```bash
export FEDIVERSE_WEBHOOK_SECRET=$(openssl rand -hex 32)
docker compose up --build
go run ./cmd/ussdsim -shortcode '*384*1234#'
```

Per-component notes:

- `gateway/` — Go service; [README](gateway/README.md) covers configuration and self-hosting. Redis is optional, needed only for more than one replica.
- `sdk/go/` — Go module; [README](sdk/go/README.md) has a whole application in one snippet.
- `adapters/fediverse/` — Go, built on the SDK. Needs `FEDIVERSE_WEBHOOK_SECRET` matching the gateway's tenant config.
- `cmd/ussdsim/` — the terminal handset. No dependencies.
- `sdk/typescript/` — not yet started (milestone M3).

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
