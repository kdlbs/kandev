# ADR-2026-10-28-pr-walkthrough-model-fallback: Discover walkthrough models and fall back to Go

**Status:** accepted
**Date:** 2026-10-28
**Area:** workflow

## Context

PR #4795's walkthrough did not generate after its selected free OpenCode model
reported a usage limit and the process remained live until the 600-second
deadline. Free models appear and disappear, and their limits may be shared.
The existing runner pins one model and does not react to a provider error.

The workflow currently specifies zero retention and no training for PR
context. OpenCode's free models have different policies, and its Go privacy
table states that Muse Spark 1.3 Contributor may use prompts and completions for
training. The user requested dynamic free models and this specific Go model.

## Decision

Discover free, tool-capable models from the `opencode` provider on each run and
try available candidates after recognized model quota, rate-limit, or
availability failures. Keep `opencode-go` metadata separate so that free models
from other providers never enter the free rotation. After all eligible free
candidates fail in that manner, or none exist, use the requested
`opencode-go/muse-spark-1.3-contributor` as the last model when the shared
deadline and credentials permit. Preserve receipt verification, process
cleanup, cancellation, advisory failure, and the shared deadline.

This explicitly changes the data-use policy for walkthrough generation. The
selected free model may retain data or use it for training, and the requested
Contributor model permits training on submitted prompts and completions. The
workflow must identify the selected model in its diagnostics and keep the
existing trusted-checkout and secret isolation boundaries.

## Consequences

- New free models can become candidates without a repository edit; changing
  provider availability or privacy terms can affect future runs.
- Free-model quotas and rate limits may be shared, so rotation does not
  guarantee success.
- The Go fallback needs a working key and subscription and may be unavailable
  by region. It incurs subscription usage or charges under the account's terms.
- Provider errors and model metadata need explicit validation. Unknown errors,
  failed discovery, cancellation, cleanup failure, and invalid output do not
  silently trigger paid generation.
- The one total deadline can expire before all candidates are attempted.

## Alternatives Considered

- Keep the zero-retention and no-training policy and use only verified models:
  this excludes the explicitly requested Contributor fallback and some dynamic
  free candidates.
- Pin another free model: its availability can change without a repository
  change, and it leaves the same single-model failure mode.
- Always use Go: this spends paid usage before trying free capacity.

## Sources

- [OpenCode Console models and privacy](https://dev.opencode.ai/v2/docs/console/models)
- [OpenCode Go models and privacy](https://dev.opencode.ai/v2/docs/console/go/)
