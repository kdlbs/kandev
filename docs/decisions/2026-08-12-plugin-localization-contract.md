# ADR-2026-08-12-plugin-localization-contract: Host-Scoped Plugin Localization

**Status:** accepted
**Date:** 2026-08-12
**Area:** frontend, protocol

## Context

Native UI plugins render inside Kandev but ship independently, so their copy cannot live in the
application's built-in catalogs. Giving plugins direct access to the shared i18next instance would
let one plugin collide with or remove another plugin's messages, and hardcoded English prevents
locale changes and pseudo-locale QA.

## Decision

The runtime-free `@kandev/plugin-sdk` contract exposes
`registry.registerTranslations(catalogs)` plus a plugin-scoped `host.i18n` API. Every catalog must
include English, may target only Kandev-supported locales, and uses bounded flat message keys. The
host mounts catalogs under a namespace derived from the verified plugin ID, supplies imperative
`t()` and reactive `useTranslation()` access, and removes resources on disable, reload, failed load,
or uninstall.

Plugin React surfaces use `host.i18n.useTranslation()` so locale changes rerender without reload.
Imperative callbacks use `host.i18n.t()` at execution time. Registration metadata that must remain
live uses accessor properties backed by `host.i18n.t()`; the registry preserves those accessors and
invalidates its subscribers when locale changes. Provider data, user content, identifiers, and
machine-readable discriminants are never translated or used as translated control-flow tokens.

## Consequences

Official and third-party plugins can follow Kandev locale and pseudo-locale behavior without
bundling i18next or importing private application modules. Catalogs remain isolated and lifecycle
safe, while English remains a guaranteed fallback. Plugin authors must maintain their own catalogs,
and human-locale parity remains advisory just like Kandev's built-in catalogs.

## Alternatives Considered

- Put official plugin copy in Kandev's built-in catalogs. Rejected because plugin releases would
  remain coupled to host releases and community plugins could not participate.
- Expose the shared i18next instance. Rejected because namespace ownership and cleanup would be
  unenforceable.
- Let each plugin bundle its own localization runtime. Rejected because locale selection, fallback,
  pseudo-locale QA, and lifecycle behavior would diverge across plugins.
