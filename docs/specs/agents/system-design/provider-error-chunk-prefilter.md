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

Adjacent literals are merged before the walk so that tokens stay whole. A
pattern with an uncovered path uses `gateTokensByRuleID`, one token per match
path. A provider-scoped runtime rule with a custom matcher leaves that provider
ungated, so every text from it is classified. If any other rule has no tokens,
the gate is off and `MayClassify` accepts every text.

Stderr and Stdout are joined as `Classify` joins them. Text containing any
non-ASCII character is accepted, because Go's case-insensitive matching folds
non-ASCII letters onto ASCII ones. Otherwise the text is lowercased and searched
for a token, both as given and after `Sanitize`, since `Classify` reads the
redacted form too and redaction can introduce placeholder literals.

A loaded operator catalog adds rules the gate does not know, so while one is
loaded `MayClassify` accepts every text.

## Structured evidence

Any non-text classifier input returns true: `StructuredErr`, `HTTPStatus`,
`ACPCode`, `ErrorKind`, `ProviderCode`, `ExitCode`,
`ManagedRuntimePackageSpec`, `ResetHint`, diagnostic keyword context, or an
injected failure for the provider.

## Call site

`convertMessageChunkWithProtocolID` classifies a message chunk only when
`MayClassify` is true; a skipped chunk has no classification and is not a
diagnostic candidate unless it continues a recognized usage-limit notice. The
agent quota-notice buffer is classified through `ClassifyAgentNotice` under the
same check.

## Verification obligations

- Every catalogue rule has gate tokens.
- Every string fixture of the `routingerr` package that `Classify` matches, for
  every provider, passes `MayClassify`, and every catalogue rule is matched by
  at least one fixture.
