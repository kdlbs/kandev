# Choosing a Kandev Image

Kandev publishes two server image flavors to GitHub Container Registry. Both run the same Kandev control plane and use `/data` for persistent state. The universal flavor adds developer tools and starts as a non-root user, which affects volume setup. The [public Docker guide](public/docker.md) owns current tags, launch commands, and runtime details.

| Version tag | Contents | Choose when |
| --- | --- | --- |
| `ghcr.io/kdlbs/kandev:X.Y.Z` | Kandev, Node 24/npm, Git, `gh`, `procps`, Python/pipx, Apprise, Azure CLI | Agent work needs only the base tools or a small explicit extension. |
| `ghcr.io/kdlbs/kandev:X.Y.Z-universal` | Base plus Go, Rust, pnpm, build tools, common developer CLIs, and Playwright Chromium system libraries | Agent work needs those toolchains or native build support. |

`:latest` and `:universal` are moving tags. Use a release version and preferably its resolved digest for a reproducible deployment. The universal moving tag also gets scheduled rebuilds between releases.

## What's in `:universal` (and what's not)

**Language toolchains:** Go at the version pinned by that release's `Dockerfile.universal`, Rust from the stable channel resolved during the image build, and pnpm installed under `/usr/local`.

**Build essentials:** `build-essential` (gcc, g++, make, libc-dev), `pkg-config`, `python3-dev`, `libssl-dev`. These cover CGO compilation, native Python pip wheels, and native Node modules.

**Linters and developer CLIs:** `golangci-lint`, `ripgrep`, `fd`, `jq`, `yq`.

**Playwright Chromium system libraries** (browsers themselves are *not* preinstalled - see below).

**Not included:**

- **JDKs / .NET / Mono.** If these matter to your workflow, derive your own image.
- **Playwright browsers** (`chromium`, `firefox`, `webkit`). Install only the browsers your task needs as the `kandev` user. Downloads under `HOME=/data/home` persist on the volume.
- **Database servers** (Postgres, MySQL, Redis). Use a sidecar or external service.
- **Kandev's own test dependencies beyond what's listed above.** Universal *is* enough to run kandev's backend Go tests, but the Playwright e2e suite needs the user to `pnpm install` and run `playwright install` first, same as on a fresh dev machine.

## Inclusion policy

We add a tool to universal when **all four** of these hold:

1. It's commonly needed by agent-driven dev workflows (Go/Rust/Python/Node project work).
2. It's reasonably available in apt or a well-maintained prebuilt binary.
3. It adds less than 200 MB to the installed image.
4. It doesn't carry a license that conflicts with redistributing the image.

For project-specific needs, derive an image as described below. Propose broadly useful tools with their workflow, installed size, architecture support, and license implications.

## Switching and extending images

The [public Docker guide](public/docker.md) owns the current run, persistence, and derived-image procedure. Keep the same `/data` volume when recreating a container with a new image. Local and Worktree agents launched by a Docker-hosted Kandev server use the **server image**; the Local Docker executor launches a separate task image.

For an optional native dependency, use the [pinned PostgreSQL-client example](../examples/docker-server-extension/Dockerfile) and its [build and disposable validation steps](public/docker.md#check-an-extension-in-a-disposable-deployment). A checkout's root `Dockerfile` consumes a prebuilt release `bundle/`; plain `docker build .` from source fails. Derive from a published version/digest to inherit the bundle and entrypoint.

The base image starts as root only to repair `/data` ownership, then the inherited entrypoint runs Kandev as `kandev` (UID 1000). Keep that startup behavior for a derived base image. Universal ends with `USER kandev`; it skips that repair, so provision ownership of a bind mount before launching it. Details and the named-volume option are in [image runtime behavior](public/docker.md#image-runtime-behavior).

### Image installs versus volume installs

Install OS dependencies and fixed agent CLI versions into the **image**, outside `/data`. A volume mounted at `/data` hides files baked there. For example, a CLI installed under `/usr/local` at build time survives container recreation as part of the image. By contrast, the Settings > Agents install action writes npm globals to `/data/.npm-global` at runtime, so they persist with the volume and can differ from the image version.

The inherited `PATH` puts `/data/.npm-global/bin` before `/usr/local/bin`. A runtime-installed CLI with the same name therefore wins over one baked into `/usr/local/bin`. Check `command -v <tool>` and `<tool> --version` in an actual Local or Worktree agent launch when diagnosing precedence. The inherited `HOME=/data/home` keeps user configuration and caches on the volume; use the service user for manual installs so those paths stay writable.

Do not silently install dependencies from project configuration at startup. Pin the base and explicit dependency inputs in a derived Dockerfile, verify build output, and rebuild when upgrading the Kandev base. A version-pinned Debian package still depends on signed repository metadata and transitive package availability; see the example's limits and validation instructions in the public guide.
