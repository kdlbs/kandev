# ADR-2026-11-03-native-web-verification: Native web verification hosts

**Status:** accepted
**Date:** 2026-11-03
**Area:** frontend, infra

## Context

The account permits 60 concurrent CI jobs.
The maintainer selected execution reductions within the existing jobs and excluded Blacksmith.
Local experiments compared current tools with native compiler and linter candidates.
The [evidence](../plans/native-web-verification/evidence.md) records their versions, timings, and compatibility findings.

The maintainer selected full Oxlint with migrated plugins and `tsrs --fast` for the next design package.
Fast mode matched the observed project diagnostics but has documented checker-history and instantiation-limit differences.
Full Oxlint still reported 37 migration findings.
Those observations do not establish a passing replacement.

The existing architecture decision selects an ESLint host for the bounded System Query guard.
A host migration must preserve that guard's syntax, four owner paths, and independent Python architecture engine.
Future work needs one durable record of this selected boundary.

## Decision

Use full Oxlint with native rules and compatible JS plugins as the target web lint host.
Use `tsrs --noEmit --fast` as the target web compiler invocation.
Keep classic TypeScript and ESLint available for their existing JavaScript API consumers.
Do not replace workspace `tsc` binaries or migrate unrelated package compilers through dependency resolution.

Both target commands require complete file coverage, effective guards, diagnostic comparison, and passing performance acceptance before cutover.
Known compatibility differences need source repairs or corrected tool releases.
Broad suppressions, weaker assertions, omitted files, and automatic success fallbacks are excluded.
The selected fast mode must not become ordinary mode silently.

This decision selects the implementation target.
It does not authorize publication, hosted dispatch, or paid capacity.
The command cutover may land as a conditional rollout with the prior commands retained as a tested rollback path;
the performance claim and removal of that rollback remain gated on three hosted acceptance pairs.
The [draft design](../specs/platform/system-design/web-native-verification.md) and [work orders](../plans/native-web-verification/plan.md) own the cutover conditions.
The earlier ESLint-host decision remains current until that cutover passes.
At cutover, its host choice is superseded by this decision.
Its bounded guard contract and Python-engine boundary remain intact.

## Consequences

The required commands can reduce execution and occupied capacity without more jobs.
The package retains classic API dependencies and adds exact native pins.
Tool upgrades require new compatibility evidence.
Fast mode cannot receive a universal conformance claim from the observed project alone.
Unresolved diagnostics or insufficient performance evidence leave the prior required commands in service.

## Alternatives Considered

- **Hybrid Oxlint and ESLint CLI:** It passed the experiment but saved less execution. The maintainer selected the complete migration.
- **Ordinary tsrs mode:** It had strong results and less diagnostic risk. The maintainer selected fast mode with explicit acceptance gates.
- **Official native TypeScript only:** It remains the reference and a viable alternative. The selected Rust compiler used less time and memory locally.
- **Replace all TypeScript dependencies:** Rejected because existing API consumers need the classic implementation.
- **Change bundlers or runners:** Excluded because the bundler already uses Rust and this package preserves current CI capacity.
