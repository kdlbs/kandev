# Tauri Desktop Signing

Public recommended desktop releases require signing on macOS and Windows. The release workflow fails closed by default when those secrets are missing.

Use `allow_unsigned_desktop=true` only for internal validation builds. That mode may upload workflow artifacts for maintainer inspection, but it does not publish a GitHub release. Unsigned desktop artifacts are internal validation only and must not be presented as trusted public downloads.

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

Linux desktop artifacts are checksum-gated. GPG/RPM signing can be added later without changing the macOS and Windows trust gate.
