# Choosing a Kandev Image

Kandev publishes two container image flavors to GitHub Container Registry. Both contain the same backend and web UI and use the same data layout. They differ in preinstalled tools and startup permissions.

These images run the Kandev server. Their tools are available to local/worktree agents inside that server container. Task Docker executors use separate agent containers with their own images. See [Docker agent environments](public/docker.md#using-docker-for-agent-environments) for executor configuration and the limitations of a containerized server.

| Tag                                     | Size (compressed) | Contents                                                                 | Pick when…                                                                                                       |
|-----------------------------------------|-------------------|--------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------|
| `ghcr.io/kdlbs/kandev:X.Y.Z`            | ~600 MB           | kandev + Node 24 + npm + git + gh + procps + python3 + pipx              | You only need npm-installable agent CLIs (claude-code, codex, …) and want the smallest possible footprint.       |
| `ghcr.io/kdlbs/kandev:X.Y.Z-universal`  | ~1.4 GB           | vanilla **+** language toolchains, build tools, Playwright Chromium deps | Your agents work on Go / Rust / Python projects, run native test suites, or drive headless browsers.             |

`:latest` aliases the vanilla image; `:universal` aliases the latest universal. **Tag pinning is strongly recommended in production** - `:latest` moves and we don't promise it stays the same forever.

## What's in `:universal` (and what's not)

**Language toolchains:** Go (latest stable), Rust (rustup, stable default toolchain), pnpm (preinstalled through npm into `/usr/local` so `pnpm` is on `$PATH` for everyone).

**Build essentials:** `build-essential` (gcc, g++, make, libc-dev), `pkg-config`, `python3-dev`, `libssl-dev`. These cover CGO compilation, native Python pip wheels, and native Node modules.

**Linters and developer CLIs:** `golangci-lint`, `ripgrep`, `fd`, `jq`, `yq`.

**Playwright Chromium system libraries** (browsers themselves are *not* preinstalled - see below).

**Not included:**

- **JDKs / .NET / Mono** - out of scope in v1; if these matter to your workflow, derive your own image (see *Customizing your image* below).
- **Playwright browsers** (`chromium`, `firefox`, `webkit`). Run `pnpm exec playwright install chromium` (or whichever browsers you need) once after the container starts. Downloads land at `~/.cache/ms-playwright`, which lives on the PV under `HOME=/data/home` and survives pod restarts. This keeps universal under 1.5 GB; it adds a one-time setup command per agent that wants browsers.
- **Database servers** (Postgres, MySQL, Redis). Use a sidecar or external service.
- **Kandev's own test dependencies beyond what's listed above.** Universal *is* enough to run kandev's backend Go tests, but the Playwright e2e suite needs the user to `pnpm install` and run `playwright install` first, same as on a fresh dev machine.

## Inclusion policy

We add a tool to universal when **all four** of these hold:

1. It's commonly needed by agent-driven dev workflows (Go/Rust/Python/Node project work).
2. It's reasonably available in apt or a well-maintained prebuilt binary.
3. It adds less than 200 MB to the installed image.
4. It doesn't carry a license that conflicts with redistributing the image.

We **decline** additions that fail any of these. The escape hatch is always *derive your own image* - see below.

## Switching between flavors

At the same release version, both flavors use the same database schema and `/data` layout. Before switching to universal, ensure the mounted data is writable by `kandev` (UID 1000). Universal does not repair volume ownership at startup. See [image runtime behavior](public/docker.md#image-runtime-behavior) for ownership requirements. For a version upgrade, also follow the [upgrade procedure](public/docker.md#upgrade-and-remove).

**Docker:**
```bash
docker pull ghcr.io/kdlbs/kandev:X.Y.Z-universal
docker stop kandev && docker rm kandev
docker run -p 38429:38429 -v kandev-data:/data ghcr.io/kdlbs/kandev:X.Y.Z-universal
```

**Kubernetes:**
```bash
kubectl set image deployment/kandev kandev=ghcr.io/kdlbs/kandev:X.Y.Z-universal
```

All your data (database, worktrees, npm globals, agent auth state) carries over because it lives on the PV, not in the image.

## Customizing your image

Universal is opinionated. If you need something it doesn't include - a JDK, a private apt repository, a specific Node version, your company's CA cert - **derive your own image** from whichever base fits.

Start from a pinned published image and use `USER root` to install tools. For a vanilla derivative, leave the final configured user as root. The inherited entrypoint repairs `/data` ownership, then uses `gosu` to run Kandev as UID 1000.

The universal recipes retain `USER kandev`. Before running them, provision `/data` and its required paths with ownership that lets `kandev` write to them. This also applies to any vanilla derivative that ends with `USER kandev`. Do not assume the primary group has GID 1000. See [image runtime behavior and persistence](public/docker.md#image-runtime-behavior) for the authoritative ownership guidance.

Save each recipe as a Dockerfile in its own build directory. Build it there, push it to your registry, and configure your deployment to use it.

### Recipe: add a JDK on top of universal

```dockerfile
FROM ghcr.io/kdlbs/kandev:0.45.0-universal

USER root
RUN apt-get update && apt-get install -y --no-install-recommends \
        openjdk-21-jdk-headless \
    && rm -rf /var/lib/apt/lists/*
# Requires writable /data ownership before startup, as described above.
USER kandev
```

```bash
docker build -t my-registry.example.com/kandev:0.45.0-jdk .
docker push my-registry.example.com/kandev:0.45.0-jdk
kubectl set image deployment/kandev kandev=my-registry.example.com/kandev:0.45.0-jdk
```

### Recipe: lightweight extras on top of vanilla

If universal is too big for you but you need a couple of specific tools:

```dockerfile
FROM ghcr.io/kdlbs/kandev:0.45.0

USER root
RUN apt-get update && apt-get install -y --no-install-recommends \
        make \
        rsync \
    && rm -rf /var/lib/apt/lists/*
```

### Recipe: bake a corporate CA cert

```dockerfile
FROM ghcr.io/kdlbs/kandev:0.45.0-universal

USER root
COPY corporate-ca.crt /usr/local/share/ca-certificates/
RUN update-ca-certificates
# Requires writable /data ownership before startup, as described above.
USER kandev
```

### Recipe: preinstall a specific agent CLI version

The Settings → Agents *Install* button calls `npm install -g` at runtime. If you want a fixed version baked into the image (so every container starts with it), do it at build time - and install **outside** `/data`, otherwise the named volume mounted at `/data` will shadow your install on every container after the first one is created:

```dockerfile
FROM ghcr.io/kdlbs/kandev:0.45.0

USER root
# Install to /usr/local (NOT /data/.npm-global, which is on the named volume
# and only seeded on first volume creation). /usr/local/bin is already on
# PATH after /data/.npm-global/bin, so a user-installed runtime version
# would still win if anyone adds one later.
RUN npm install -g --prefix /usr/local @anthropic-ai/claude-code@1.2.3
```

Note: kandev users will see the agent as already installed on the agents page. Baking it in means the version is tied to your image rather than to user choice - if a user runtime-installs a different version, that one wins (because the runtime path `/data/.npm-global/bin` precedes `/usr/local/bin` on `$PATH`).

## What if I need something that doesn't fit any of these patterns?

Two reasonable paths:

1. **Open an issue** suggesting an addition to universal. Cite (a) the workflow that needs it, (b) the installed size, (c) whether it's apt-available or needs a prebuilt binary. We'll evaluate against the inclusion policy.
2. **Just maintain your own derived image.** This is what most teams end up doing for org-specific needs (corporate CAs, internal tools, JDK pinning). It's a 10-line Dockerfile and a 2-line CI step.

## A note on building locally

The root `Dockerfile` packages prebuilt release artifacts. A plain `docker build .` from the checkout cannot build it. Follow the [release build context instructions](public/docker.md#published-images) for the required full bundle and entrypoint.

`Dockerfile.universal` can build from a published base image without preparing that release context. From the repository root, replace `X.Y.Z` with a real release:

```bash
docker build -f Dockerfile.universal \
  --build-arg BASE_IMAGE=ghcr.io/kdlbs/kandev:X.Y.Z \
  -t kandev:dev-universal .
```

To extend a locally packaged base, substitute its tag for `BASE_IMAGE`. The resulting universal image has the same volume-ownership prerequisites as the published universal flavor.
