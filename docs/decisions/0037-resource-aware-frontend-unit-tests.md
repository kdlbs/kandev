# 0037: Resource-Aware Frontend Unit Tests

**Status:** accepted
**Date:** 2026-07-14
**Area:** frontend, infra

## Context

The web unit suite contains hundreds of Vitest files and is commonly run by several concurrent Kandev tasks on the same developer host. Vitest's uncapped run mode uses all available parallelism, so overlapping full-suite runs saturate CPU, consume multiple gigabytes of memory, and can thermally throttle smaller hosts. The test stack also remained on Vitest 1 with its private Vite 5 and Tinypool dependencies while the application had moved to Vite 8.

On a 10-logical-CPU development host, the old uncapped suite averaged roughly 7.5–8 logical CPUs and peaked near 2.25 GB RSS. After the upgrade, a 20-percent cap completed all 4,661 tests using an average of 2.64 logical CPUs and a 1.72 GB peak RSS. These measurements are workload-specific, but they establish the intended order of magnitude for the local default. Vite 8.1.4's Rolldown update also misbundles `rehype-sanitize`'s re-exported `defaultSchema`, crashing the production app at startup, so its known-good 8.0.16 lock is retained pending an upstream fix.

## Decision

Kandev keeps Vitest as the frontend unit-test runner and aligns it with the application's current Vite major. Local full-suite runs use the worker-thread pool and at most 20 percent of the host's available parallelism. `pool: "threads"` is explicit because Vitest 2+ defaults to `forks`; the suite's isolation trial showed that `forks` produces widespread cross-file mock and DOM failures. CI remains uncapped so dedicated runners can use their assigned capacity, and `VITEST_MAX_WORKERS` can override either default for an explicit execution environment when it is a positive integer or percentage. Vitest is upgraded to 4.1.10; Vite remains locked at 8.0.16 rather than the incompatible 8.1.4.

The unit-test environment disables Happy DOM child-frame navigation and intercepts otherwise unmocked Happy DOM requests with a deterministic non-success response, so component tests cannot make real requests to its default `http://localhost:3000` origin. Tests that explicitly replace `fetch` retain full control over their response. The Vitest config also clears the generic inherited `DEBUG=1` value, which otherwise enables Tailwind's per-file transform diagnostics; namespaced debug values remain available for intentional tooling diagnostics.

Targeted test-file runs remain the normal development loop. Full frontend suites run during final verification and CI rather than after every edit. Per-file isolation remains enabled because the suite relies on module and DOM isolation.

## Consequences

- Concurrent Kandev tasks leave CPU capacity for the backend, agents, editor, and other test suites.
- A local full-suite run takes longer when it is the only workload, but overlapping runs cause less contention and thermal pressure.
- CI performance is unchanged unless the CI environment explicitly sets `VITEST_MAX_WORKERS`.
- Vitest and Vite share a supported dependency generation instead of loading a second legacy Vite toolchain.
- Unit tests do not load iframe content, access the network through Happy DOM, or inherit the host's generic debug verbosity.
- The percentage limit scales across developer machines without hard-coding a workstation-specific core count.
- Vite updates require a production-build smoke check until the Rolldown re-export regression is fixed upstream.

## Alternatives Considered

### Keep uncapped Vitest locally

Rejected because the fastest isolated run becomes slower and less predictable when several Kandev tasks independently consume the whole host.

### Disable test-file isolation

Rejected because a trial produced widespread cross-file mock and DOM failures. The suite depends on isolation for correctness.

### Migrate to Rstest

Deferred because its Rust-powered transform pipeline does not remove the need for a worker budget, while migration would require broad changes to Vitest imports and mocking APIs. It can be benchmarked separately after the lower-risk runner upgrade and resource policy land.
