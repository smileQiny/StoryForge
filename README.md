# StoryForge

StoryForge is a Studio-first long-form fiction generation system. It packages the Web Studio, genre library, agent orchestration, truth files, run traces, and review workflow into a single Go binary.

## Status

- Single Go HTTP server with embedded Web Studio assets.
- Filesystem-backed books, chapters, truth files, runtime state, and traces.
- Multi-agent writing pipeline with planning, composition, writing, auditing, revision, observation, and reflection stages.
- Configurable LLM profiles and per-agent model routing.

## Features

- Book creation workflow for short-form and long-form fiction projects.
- Chapter queue with preview, editing, audit, revision, rewrite, approval, and export actions.
- Truth-file management for current state, particle ledger, hooks, summaries, subplots, emotional arcs, and character matrix.
- Daemon mode for continuously scheduling eligible book/chapter work.
- Run traces and stage-level observability for agent execution.
- LLM profile support for OpenAI-compatible, Responses-style, and Anthropic-style providers.
- Built-in Web Studio served from the same binary as the API server.

## Quick Start

```bash
make build
./bin/storyforge
```

After startup, open:

- `http://127.0.0.1:8080/`
- `http://127.0.0.1:8080/healthz`
- `http://127.0.0.1:8080/api/bootstrap`

## Frontend Development

```bash
make frontend-install
make run
cd web/frontend && npm run dev
```

The Vite dev server listens on `4567` and proxies `/api` to the Go server on `8080`.

## Packaging

```bash
make build
```

The build command:

1. Installs frontend dependencies if needed.
2. Runs the Vite production build into `web/frontend/dist`.
3. Embeds `web/frontend/dist` and `genres` via Go `embed`.
4. Builds `bin/storyforge`.

The resulting binary includes the Web Studio and genre fixtures. Runtime data is not embedded and is written under `./data` relative to the process working directory.

For release artifacts:

```bash
make release
```

This writes platform archives and `checksums.txt` to `dist/`. Those files are intended to be uploaded to GitHub Releases.

## Distribution

GitHub Releases is the only trusted distribution source for StoryForge artifacts. Do not install binaries, archives, checksums, container images, or package-manager manifests from unofficial mirrors unless they are explicitly referenced by the StoryForge project.

Install the latest GitHub Release with:

```bash
curl -fsSL https://raw.githubusercontent.com/smileQiny/StoryForge/main/install.sh | sh
```

See [`DISTRIBUTION.md`](./DISTRIBUTION.md) for the release and installation policy.

## Licensing

StoryForge uses a dual-license model:

- Open-source use: GNU Affero General Public License v3.0 or later.
- Commercial use: available only under a separate written commercial license.

See [`LICENSE.md`](./LICENSE.md) and [`COMMERCIAL-LICENSE.md`](./COMMERCIAL-LICENSE.md).

## Release Automation

Pushing a version tag such as `v1.0.0` runs the GitHub release workflow. The workflow builds platform archives, generates `checksums.txt`, and publishes them to GitHub Releases.
