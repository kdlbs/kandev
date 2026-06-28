# Tauri Desktop Signing

Public recommended desktop releases require signing on macOS and Windows. The release workflow fails closed by default when those secrets are missing.

Use `allow_unsigned_desktop=true` only when intentionally producing unsigned macOS/Windows desktop artifacts. Unless `desktop_validation_only=true` is also selected, that mode still publishes the normal release outputs: the release PR and tag, GitHub release, npm packages, Homebrew update, public container tags, and desktop artifacts. Unsigned desktop artifacts may require manual OS security bypasses and must not be presented as trusted downloads.

Use `desktop_validation_only=true` for maintainer inspection builds. That mode uploads workflow artifacts but does not publish a GitHub release, npm packages, Homebrew updates, or public container tags.

## macOS

Required signing secrets:

- `APPLE_CERTIFICATE`: base64 `.p12` Developer ID Application certificate.
- `APPLE_CERTIFICATE_PASSWORD`: export password for the `.p12`.
- `KEYCHAIN_PASSWORD`: temporary CI keychain password.

Required notarization secrets, choose one path:

- Apple ID path: `APPLE_ID`, `APPLE_PASSWORD`, `APPLE_TEAM_ID`.
- App Store Connect API path: `APPLE_API_KEY`, `APPLE_API_ISSUER`, `APPLE_API_KEY_P8`.

Optional:

- `APPLE_PROVIDER_SHORT_NAME` when the Apple ID belongs to multiple provider teams.

## Windows

Required signing secrets:

- `WINDOWS_CERTIFICATE`: base64 `.pfx` code signing certificate.
- `WINDOWS_CERTIFICATE_PASSWORD`: export password for the `.pfx`.

Optional:

- `WINDOWS_TIMESTAMP_URL`: timestamp server, defaults to `http://timestamp.digicert.com`.
- `WINDOWS_SIGNTOOL_PATH`: custom `signtool.exe` path.

Linux desktop artifacts are checksum-gated. The x64 `.deb`/`.rpm` artifacts are built on Ubuntu 22.04 for an older glibc baseline. The arm64 artifacts use GitHub's Ubuntu 24.04 arm64 runner baseline. GPG/RPM signing can be added later without changing the macOS and Windows trust gate.
