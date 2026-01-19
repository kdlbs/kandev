# Releases

This repo ships two things:
- **Kandev runtime bundles** (backend + web) published to GitHub Releases.
- **NPX launcher** (`kandev` on npm) that downloads and runs those bundles.

## Packaging the web bundle

The release workflow calls:

```bash
scripts/release/package-web.sh
```

It packages the Next.js standalone output into `dist/web/`, which is then bundled into each platform ZIP.

## Release flow (runtime bundles)

1) Pick a version tag:

```bash
git tag v0.1.0
git push origin v0.1.0
```

Or use the helper script (creates and pushes the tag):

```bash
scripts/release/publish-release.sh 0.1.0
```

2) GitHub Actions will:
   - Build the web app in standalone mode.
   - Build `kandev` + `agentctl` binaries for each platform.
   - Package and upload `kandev-<platform>.zip` and `.sha256` to the release.

## Publish the NPX launcher

The launcher package lives at `apps/cli` and is published as `kandev`.

```bash
cd apps/cli
pnpm build
npm publish --access public
```

Or use the helper script:

```bash
scripts/release/publish-launcher.sh 0.1.0
```

## Releasing a new Kandev version

1) Update versions:
   - Bump `apps/cli/package.json` (npm package version).
   - The `publish-release.sh` script will error if this doesn't match the tag.
2) Tag and push the release (`vX.Y.Z`).
3) Publish the launcher to npm (same version or higher).
4) Verify:
   - `npx kandev@latest` downloads the new release bundle.
   - `npx kandev` prompts to update if an older launcher is installed.

## Notes

- Release assets are downloaded to `~/.kandev/bin/<version>/<platform>/`.
- Local data is stored under `~/.kandev/data/`.
