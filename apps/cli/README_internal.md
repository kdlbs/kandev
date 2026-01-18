# Kandev Launcher (npx)

This package powers `npx kandev` by downloading prebuilt release bundles from GitHub Releases and running them locally. It:
- Detects OS/arch and fetches the matching bundle ZIP.
- Verifies the SHA256 checksum when available.
- Extracts the bundle into `~/.kandev/bin/<version>/<platform>/`.
- Starts the backend binary and waits for the `/health` endpoint.
- Starts the Next.js standalone server with runtime `KANDEV_API_BASE_URL`.
- Uses the latest GitHub Release by default, so runtime bundles update automatically.
It also supports local dev runs from a repo checkout.

## Updates

On `run`, the launcher checks npm for the latest `kandev` CLI version and prompts:

```
Update available: <current> -> <latest>. Update now? [y/N]
```

If you accept, it re-runs `npx kandev@latest` with the same arguments.
This check is skipped in `dev` mode.

Note: the runtime bundles are pulled from the latest GitHub Release by default, even if the CLI version is unchanged. So:
- **New runtime release without CLI publish**: users get new runtime automatically, but no update prompt.
- **New CLI publish**: users get an update prompt and then re-run with the new CLI.

You can disable the prompt with:

```bash
KANDEV_NO_UPDATE_PROMPT=1 npx kandev
```

## Usage

```bash
# Run the latest release bundle
npx kandev

# Run a specific release tag
npx kandev run --version v0.1.0

# Local dev (from repo root)
npx kandev dev


# Local test the built CLI (from repo root)
pnpm -C apps/cli build
pnpm -C apps/cli start
```

## Local Development

```bash
pnpm -C apps/cli dev
```

## Build / Publish

```bash
pnpm -C apps/cli build
npm publish --access public
```

The published package name is `kandev`, with a bin entry `kandev`.

## Release

```bash
scripts/release/publish-launcher.sh 0.1.0
```

## Environment Overrides

- `KANDEV_GITHUB_OWNER`, `KANDEV_GITHUB_REPO`: Override the GitHub repo to fetch releases from.
- `KANDEV_GITHUB_TOKEN`: Optional token for GitHub API rate limits.
- `KANDEV_NO_UPDATE_PROMPT=1`: Disable the update prompt.
- `KANDEV_SKIP_UPDATE=1`: Internal guard to avoid update loops.
