# Tauri Desktop Signing

The release workflow signs desktop artifacts opportunistically. Complete macOS and Windows signing inputs produce signed/notarized artifacts. Missing or incomplete inputs do not block the release; those platform artifacts are built unsigned and the GitHub release notes get an unsigned-artifact warning.

Unsigned desktop artifacts may require manual OS security bypasses and must not be presented as trusted downloads.

Use `desktop_validation_only=true` for maintainer inspection builds. That mode uploads workflow artifacts but does not publish a GitHub release, npm packages, Homebrew updates, or public container tags.

## macOS

Signing inputs:

- `APPLE_CERTIFICATE`: base64 `.p12` Developer ID Application certificate.
- `APPLE_CERTIFICATE_PASSWORD`: export password for the `.p12`.
- `KEYCHAIN_PASSWORD`: temporary CI keychain password.

Notarization inputs, choose one path:

- Apple ID path: `APPLE_ID`, `APPLE_PASSWORD`, `APPLE_TEAM_ID`.
- App Store Connect API path: `APPLE_API_KEY`, `APPLE_API_ISSUER`, `APPLE_API_KEY_P8`.

Optional:

- `APPLE_PROVIDER_SHORT_NAME` when the Apple ID belongs to multiple provider teams.

## Windows

Signing inputs:

- `WINDOWS_CERTIFICATE`: base64 `.pfx` code signing certificate.
- `WINDOWS_CERTIFICATE_PASSWORD`: export password for the `.pfx`.

Optional:

- `WINDOWS_TIMESTAMP_URL`: timestamp server, defaults to `http://timestamp.digicert.com`.
- `WINDOWS_SIGNTOOL_PATH`: custom `signtool.exe` path.

Linux desktop artifacts are checksum-gated. The x64 `.deb`/`.rpm` artifacts are built on Ubuntu 22.04 for an older glibc baseline. The arm64 artifacts use GitHub's Ubuntu 24.04 arm64 runner baseline. GPG/RPM signing can be added later without changing the macOS and Windows trust gate.
