# 0040: Separate Updater Integrity from OS Publisher Identity

**Status:** accepted
**Date:** 2026-07-15
**Area:** desktop, infra, workflow

## Context

Kandev can sign every Tauri updater payload with its dedicated updater key before Apple Developer
and Windows code-signing credentials are available. Blocking updater publication on those OS
credentials would leave users of the currently distributed unsigned applications without the
integrity and recovery benefits of the in-app updater.

## Decision

The release workflow requires `TAURI_SIGNING_PRIVATE_KEY` to produce updater artifacts on every
desktop target. It publishes a complete `latest.json` only when all five target payloads exist and
verify cryptographically against the public key embedded in the application.

Apple Developer ID signing/notarization and Windows code signing are independent publisher-identity
layers. Missing OS credentials do not block Tauri-signed updater artifacts or the update feed. The
corresponding installers and updater payloads remain explicitly identified as unsigned development
builds in release notes until those credentials are configured.

## Consequences

Users of an installed unsigned build can receive payloads whose integrity and Kandev origin are
verified by the updater key. Initial installation and OS trust prompts remain unchanged: macOS and
Windows can still warn that the publisher is unidentified. The updater private key becomes a
critical long-lived release credential because losing or rotating it without a migration prevents
existing installations from accepting later updates.

Adding Apple or Windows credentials later requires no updater protocol change. The same release
workflow will add OS identity signatures while continuing to use the existing Tauri updater key.

## Alternatives Considered

- **Block all updater artifacts until Apple and Windows signing are configured.** Rejected because
  it couples payload integrity to credentials that are not currently available and removes updates
  from every platform.
- **Publish only Linux updater entries.** Rejected because it creates a partial feed and leaves the
  currently distributed macOS and Windows applications without an updater path.
- **Publish updater payloads without Tauri signatures.** Rejected because the Tauri updater requires
  cryptographic signatures and Kandev must fail closed on missing or invalid payload signatures.
