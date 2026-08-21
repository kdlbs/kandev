# ADR-2026-08-20-settings-prompt-editor-provider-ownership: Settings Prompt Editors Own Monaco Completion Providers

**Status:** accepted
**Date:** 2026-08-20
**Area:** frontend

## Context

Monaco registers completion providers by language, but Settings can mount more
than one prompt editor with different placeholder and saved-prompt catalogs.
The previous language-wide singleton allowed the last mounted editor to replace
the provider used by another open editor. Saved-prompt catalogs also load
asynchronously, so registration must handle data arriving before or after
Monaco mounts.

## Decision

Each `ScriptEditor` instance owns its primary and saved-prompt completion
registrations. The registrations wrap their providers with a model-identity
check and return no suggestions for another model. Provider disposables are
updated and cleaned up per instance. The shared `SettingsPromptEditor` enables
saved-prompt registration only for fields whose runtime expands `@name`
references, and its completion widget is layered above the mobile Settings Save
bar.

## Consequences

Open editors no longer share mutable provider state, so placeholder and saved-
prompt suggestions remain local to the editor that requested them. Each editor
performs its own registration and disposal work, and provider changes replace
only that editor's registration. The shared prompt editor must retain the
latest registration callback because prompt data may load before Monaco's
one-shot mount callback.

## Alternatives Considered

- Keep one language-wide provider per slot. This is simpler, but it reintroduces
  cross-editor catalog leakage and makes unmount order observable.
- Keep a global provider and route requests through the active editor. This
  requires focus and model bookkeeping outside Monaco and is fragile when
  several editors are mounted or a completion request races a focus change.
- Give every editor a distinct Monaco language identifier. This avoids some
  collisions, but multiplies language registrations and does not improve the
  provider lifecycle or async catalog handling.
