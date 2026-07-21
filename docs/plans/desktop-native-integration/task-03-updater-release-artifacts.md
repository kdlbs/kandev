---
id: "03-updater-release-artifacts"
title: "Signed updater release artifacts"
status: done
wave: 2
depends_on: ["01-native-shell-commands"]
plan: "plan.md"
spec: "../../specs/desktop-tauri-app/spec.md"
---

# Task 03: Signed Updater Release Artifacts

## Acceptance

- Release builds enable Tauri updater artifacts and publish signed platform bundles plus
  `latest.json` for macOS arm64/x64, Linux x64/arm64 AppImage, and Windows x64 NSIS.
- Existing macOS DMG, Linux AppImage/DEB/RPM, and Windows installer artifacts remain published.
- The manifest contains version, notes/date, target URLs, and inline signatures using the
  dedicated Tauri updater key; missing signatures or target artifacts fail updater verification.
- Updater private keys remain CI secrets and are distinct from OS signing credentials.
- OS-unsigned development installers may still publish and participate in the updater when their
  updater payloads have valid Tauri signatures. Payloads without Tauri signatures are never
  included in `latest.json` or offered by the in-app updater.
- Release verification recognizes updater archives/signatures instead of dropping them during
  collection.

## Files Likely Touched

- `.github/workflows/release.yml`
- `scripts/release/verify-desktop-assets.sh`
- New updater manifest generation/verification script under `scripts/release/`
- Existing release configuration tests under `apps/cli/src/`
- Desktop release/signing documentation

## Verification

```bash
rtk bash -n scripts/release/verify-desktop-assets.sh
cd apps && rtk pnpm --filter @kandev/cli test -- --run release
```

Add fixture tests for all target keys, missing/invalid signatures, wrong URLs, prerelease metadata,
and retention of `.app.tar.gz`, `.AppImage.tar.gz`, NSIS updater bundles, and `.sig` files.

## Output Contract

Record required CI secret names without values, update this task to `done`, and check its plan item
only after fixture-based manifest verification passes.

## CI Secrets

Updater publication requires `TAURI_SIGNING_PRIVATE_KEY`. If the updater key is encrypted, also
configure `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`. These are distinct from optional macOS and Windows
OS-signing credentials.
