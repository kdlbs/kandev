# Native web verification investigation

## Provenance and scope

The investigation used source revision `8cd31a0896abc50e6d9683369cee92252c5d2e44` on November 3, 2026.
It copied the complete source and generated inputs into an isolated temporary project.
All tracked source bytes matched the original after the experiments.
Repository dependencies, commands, workflows, and permanent tests remained unchanged.

The [curated baseline](baseline-2026-11-03.json) records exact pins, samples, inventories, findings, and outcome limits.
Raw logs and the temporary harness remain outside version control.
The implementation must not depend on `/tmp/kandev-toolbench` surviving.
Task 01 creates the reproducible evidence helper.

The host ran Node 24.16.0 on Linux x64 with 11 available CPUs.
Each benchmark used the same four-CPU affinity and an 8 GiB Node heap limit.
Commands ran sequentially on a shared host.
The investigation did not flush operating-system caches.
Initial incomplete-snapshot smoke checks failed and are excluded from comparison medians.

## Compiler observations

Three non-incremental runs per compiler used the same configured project and 7,635 non-dependency files.
The second round reversed compiler order.
The different compilers retained their own bundled standard libraries.

| Compiler | Median execution (s) | Median peak RSS (MiB) | Project result |
| --- | ---: | ---: | --- |
| TypeScript 5.9.3 | 90.470 | 4927.270 | Passed |
| TypeScript 7.0.2 | 16.645 | 5204.672 | 12 errors |
| TypeScript 7.1.0-beta | 10.955 | 3936.750 | 12 errors |
| tsrs 0.20.1 default | 4.370 | 1542.266 | Same 12 reference errors |
| tsrs 0.20.1 fast | 3.609 | 1123.832 | Same 12 reference errors |

The tsrs package was `0.20.1-ts7.1.0-beta`, pinned to upstream compiler commit `3e9236361fa9`.
Default and fast modes matched the observed reference diagnostic locations, codes, and messages.
All five modes detected eight injected type errors at matching locations and codes.
Those cases do not establish universal compiler conformance.

A separate single control preserved `incremental: true` with a new cache per compiler.
TypeScript 5 took 74.284 seconds.
Default tsrs took 3.668 seconds, and fast tsrs took 4.053 seconds.
That control does not establish that fast mode consistently beats default mode.
The maintainer selected fast mode as the delivery target after reviewing these results.

The 12 findings need compatibility repairs before cutover:

- Seven Observer-mock diagnostics concern the missing `scrollMargin` member across four files.
- Four WebSocket-mock diagnostics concern the closed `readyState` type.
- One terminal resize diagnostic concerns a declared `Uint8Array<ArrayBufferLike>` value at `WebSocket.send`.

The baseline JSON names all six affected files and their original locations.
Future discovery must refresh those locations before edits.

## Linter observations

The investigation compared the same 9,806 original source files.
Scratch helper files are excluded from inventory equality.
ESLint 9.39.2 was the baseline.
The candidate used Oxlint 1.91.0 after conversion with `@oxlint/migrate` 1.90.0.

| Configuration | Median execution (s) | Samples | Result |
| --- | ---: | ---: | --- |
| Existing full ESLint | 82.487 | 3 | Passed |
| Full Oxlint with migrated JS plugins | 19.803 | 3 | 37 additional findings |
| Oxlint plus compatible ESLint coverage | 63.242 | 1 | Passed |

Automatic conversion did not preserve override exclusions or register local plugins correctly.
The temporary configuration repaired those scopes and registrations before the final full-candidate measurements.
Nine focused native-host guard cases passed.
The hybrid also passed eleven combined guard and exemption cases.

The remaining full-candidate findings were:

- 23 i18next reports on punctuation entities such as `&middot;`, `&rarr;`, `&ldquo;`, and `&rdquo;`.
- 12 optional-chaining reports around TypeScript casts in fixtures.
- Two duplicate-method parser errors in `ApiClient.createSentryInstance` and `SessionPage.prStatusChip`.

The baseline JSON preserves the 37 file, line, rule, and message records.
Full Oxlint still failed, so its timing does not establish accepted migration savings.
The hybrid passed with original guards retained, but the maintainer selected the complete migration.

## Bundler and API boundaries

The existing Vite 8.0.16 and Rolldown 1.0.3 build passed in 13.378 seconds.
The package retains this Rust-based bundler.
It does not introduce Rspack, another transformer, or a different test bundler.

TypeScript 5 exposes the JavaScript compiler API that current lint tools use.
The native TypeScript 7 package does not provide that compatible API.
The native CLI migration therefore retains classic TypeScript dependencies and isolates reference installations.

## Implementation evidence

The implementation ran on November 3, 2026 on the repaired source.

### Compiler comparison

`python3 .github/scripts/web-toolchain-evidence.py compare-types --web-root apps/web --mode fast`
provisioned TypeScript `7.1.0-beta` in an isolated temporary installation and compared it with the
workspace `tsc` 5.9.3, tsrs default, and `tsrs --fast` on the same 7,581-file non-dependency input
set. All four compilers passed with zero project diagnostics in cold and fresh incremental runs,
and every tool reported an equal input inventory. The project comparison used the supported
four-checker setting; corpus comparisons also passed at one, four, and eight checkers. Each tool used its own temporary `tsBuildInfoFile`,
so neither the reference nor either tsrs mode could read another compiler's cache. The comparison
passed `--checkers 4` to both the pinned reference and tsrs; classic TypeScript 5 kept its unsupported
CLI untouched.

Diagnostics are compared as complete normalized tuples: file, line, column, code, severity, and
message, with multiplicity. A changed location, column, or severity therefore fails even when the
message text is unchanged. An owned incremental probe ran each tool through a clean cold pass, a
fresh warm pass, and a changed-invalid warm pass; every tool reported the injected TS2322 error and
none reused a stale success.

The required corpus is declared by `types/corpus.json` and asserts expected clean and error outcomes,
including required diagnostic codes. The invalid project reported eight diagnostics in the reference,
default tsrs, and fast tsrs at matching tuples. The valid project reported zero. The instantiation
project reported the same TS2589 diagnostic in every compiler and mode, while its bounded counterpart
stayed clean. A missing corpus, an undeclared project, or a formerly negative project that reports
zero diagnostics fails the comparison. The final cold benchmark measured a median complete typecheck
of 110.246 seconds for the workspace compiler and 6.103 seconds for fast tsrs; see the adoption
results below.

### Lint comparison

`python3 .github/scripts/web-toolchain-evidence.py compare-lint --web-root apps/web
--config apps/web/oxlint.config.mjs` reported equal 9,810-file inventories and zero findings from
both the retained ESLint host and the native candidate. The diagnostic comparison samples ranged
from 97 to 117 seconds for ESLint and 20 to 29 seconds for Oxlint under shared-host load. The final
cold benchmark measured medians of 88.592 and 30.461 seconds.

The 37 candidate findings were resolved without exclusions: 23 JSX entities became the same
punctuation characters in Unicode form, 12 unsafe optional-chain expressions were rewritten with
explicit callback or state guards, and the two shadowed `ApiClient.createSentryInstance` and
`SessionPage.prStatusChip` declarations were removed while retaining the effective later
implementations. `rule-ledger.json` records all 96 configured rule registrations, severities, options,
scopes, exemptions, and native targets, plus the disabled rules and the native-parser `no-octal`
coverage.

The native wiring suite executes the required CLI against the real configuration and asserts all
four System owner paths, all four Query rules, i18n and task-link guards, E2E waits, exemptions,
inline suppressions, parser errors, a missing plugin, and paths containing spaces.

The evidence helper rejects truncated stdout or stderr, non-path inventory lines, failure exits
without parsed diagnostics, diagnostics on a success exit, and truncated inventories. Failed runs
still write a partial report with every attempted command, so an early missing binary or version
mismatch cannot erase the evidence record.

### Local adoption measurement

`python3 .github/scripts/web-toolchain-evidence.py benchmark --web-root apps/web --pairs 3`
alternated complete lint and typecheck command sequences. Each compiler sample started from a cold
incremental cache, which the helper archives outside the working tree before every side, so the
baseline and candidate could not read each other's build info. Application source stayed identical
across all six sides.

| Measurement | Baseline median (s) | Candidate median (s) | Reduction |
| --- | ---: | ---: | ---: |
| Full lint | 88.592 | 30.461 | 65.62% |
| Complete typecheck | 110.246 | 6.103 | 94.46% |
| Combined occupied execution | 198.838 | 36.564 | 81.61% |

All three pairs passed. Peak process RSS was about 5.0-5.4 GiB for the workspace compiler and
1.5 GiB for fast tsrs; the measurement scope is each direct child process, not aggregate job memory.
Shared-host load produced wide absolute variance, so these numbers establish the local bounds and
not a hosted claim. Hosted evidence remains outstanding; Task 05 therefore stays open.

## Acceptance limits

These local samples identify candidates and compatibility work.
They do not establish hosted performance, complete-command savings, or production adoption.
The new package requires passing source repairs, stronger regression evidence, and three comparable acceptance pairs.
Waiting under the 60-job limit remains an accepted operational constraint.
No queue reduction is claimed or required by this package.

## Primary sources

- [tsrs project and release scope](https://github.com/oxc-project/tsrs).
- [tsrs fast-mode side effects](https://github.com/oxc-project/tsrs/blob/main/docs/USAGE.md#side-effects).
- [tsrs differences from its pinned compiler](https://github.com/oxc-project/tsrs/blob/main/docs/DEFAULTS.md).
- [Oxlint migration and JS plugins](https://oxc.rs/docs/guide/usage/linter/migrate-from-eslint.html).
- [Native TypeScript and API compatibility](https://devblogs.microsoft.com/typescript/announcing-typescript-7-0/).
