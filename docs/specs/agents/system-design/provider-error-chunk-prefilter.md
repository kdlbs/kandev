---
status: draft
system: agents
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-PREFILTER-001
---

# Provider Error Classification Pre-Check on Message Chunks System Design

## Purpose and boundaries

`routingerr.MayClassify` answers whether `Classify` could return anything but
its default for an input, without running the rule engine. The ACP adapter
calls it before classifying a message chunk for the diagnostic-candidate
marker. Other `Classify` callers are unchanged.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-PROVIDER-ERROR-PREFILTER-001` | [Gate tokens](#gate-tokens), [Structured evidence](#structured-evidence), [Call site](#call-site) |

## Gate tokens

At package initialization the gate parses every provider, provider-neutral,
and runtime-environment rule pattern with `regexp/syntax` and computes the
literals each match must contain, as a disjunction over alternation paths:

- a literal contributes its longest ASCII alphanumeric run of at least four
  characters;
- a concatenation crosses the requirements of its elements;
- an alternation needs every branch covered, otherwise the pattern has no
  derived tokens;
- an optional element contributes nothing.

Adjacent literals are merged before the walk so that tokens stay whole. A rule
with a custom matcher, or a pattern with an uncovered path, uses
`gateTokensByRuleID`, one token per match path. If any rule has neither, the
gate is off and `MayClassify` accepts every text.

Text is lowercased before the token search, and U+017F is folded to `s`, so
case-insensitive patterns cannot match text the gate rejects. Stderr and Stdout
are checked separately; tokens contain no newline, so the joined text used by
`Classify` adds no match.

## Structured evidence

Any non-text classifier input returns true: `StructuredErr`, `HTTPStatus`,
`ACPCode`, `ErrorKind`, `ProviderCode`, `ExitCode`,
`ManagedRuntimePackageSpec`, `KeywordContext.Enabled`, or an injected failure
for the provider.

## Call site

`convertMessageChunkWithProtocolID` sets `ProviderDiagnosticCandidate` through
`isProviderDiagnosticCandidate`, which returns false when `MayClassify` is
false and otherwise applies the existing high-confidence, fallback-allowed test
to `Classify`.

## Verification obligations

- Every catalogue rule has gate tokens.
- Every string fixture of the `routingerr` package that `Classify` matches, for
  every provider, passes `MayClassify`, and every catalogue rule is matched by
  at least one fixture.
