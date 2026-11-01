---
id: "01-match-provider-signatures"
title: "Match provider limit signatures instead of topic words"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROVIDER-ERROR-SIGNATURES-001
acceptance_criteria:
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.1
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.2
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.3
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.4
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.5
  - AC-AGENTS-PROVIDER-ERROR-SIGNATURES-001.7
system_design:
  - ../../specs/agents/system-design/provider-error-signatures.md
---

# Task 01: Match provider limit signatures instead of topic words

## Summary

Replace the topic-word rate, quota, and subscription patterns in the provider
rule table with provider error signatures, keeping rule order, confidence, and
rule IDs.

## Changes

- `claude.stderr.rate.v1`: Anthropic 429 API errors, `rate_limit_error`,
  `429 Too Many Requests`, "you've hit your rate limit", and a line-opening
  "rate limit exceeded/reached".
- `claude.stderr.quota.v1`: word-bounded `anthropic_quota_exceeded`, "credit
  balance is too low", "insufficient credits".
- `claude.stderr.subscription.v1`: missing, inactive, expired, or required
  subscription wording.
- `opencode`, `copilot`, `amp` rate rules: shared rate-limit signature.
- `opencode`, `amp` quota rules: shared quota signature.

## Acceptance

1. Agent prose about rate limits, quotas, credit balances, and subscriptions
   classifies as none of `rate_limited`, `quota_limited`, or
   `subscription_required` for `claude-acp`, `opencode-acp`, `copilot-acp`, and
   `amp-acp`, in the prompt-send and streaming phases.
2. Claude prose that quotes another provider's "Rate limit exceeded"
   mid-sentence is not a Claude rate limit.
3. The listed provider signatures still classify as before, and the existing
   classifier tests pass unchanged.

## Verification

- `go test ./internal/agent/runtime/routingerr/ -run 'TestClassify_AgentProseAboutLimitsIsNotAProviderLimit|TestClassify_ProviderLimitSignaturesStillClassify'`
- `go test ./internal/agent/runtime/routingerr/`
- `make -C apps/backend build`

## Context-length producer/consumer follow-up

The terminal stream projection also keeps execution identity separate from the
agent type used for classification. A real-handler disposable check supplies
the lifecycle producer's actual field shape: execution ID in `AgentID`, type in
`AgentType`, and an OpenCode Go provider diagnostic. The typed case must use the
existing quota skip action immediately, while legacy type fallback and the
output/effect/stale fences remain. No duplicate Go regex or permanent test is
added.

The completed scope and verification above are preserved. Add only the captured
named context-length rule at prompt-send and its existing exact terminal-
provider consumer compatibility. Preserve typed-limit priority, other phase
classifications, false global retry/fallback flags and ordinary manual recovery.
Other signature/catalogue/notice work remains with its existing owner.

Changed source paths:

- `apps/backend/internal/agent/runtime/routingerr/provider_neutral_rules.go`
- `apps/backend/internal/agent/runtime/routingerr/routingerr.go`
- `apps/backend/internal/orchestrator/event_handlers_transient_messages.go`

The linked requirement, design, this work order and its plan cover this coherent
producer/consumer unit. No endpoint, schema, setting, permission step, persistent
test or operating mutation is included.

Disposable checks called real `routingerr.Classify` and `classifyKanbanFailure`
entrypoints. The baseline lacked the named rule (RED, 0.023s). The producer-only
candidate exposed the original consumer mismatch (RED, 0.062s). The complete
candidate passed the named-rule contract, exact conversion, startup/streaming/
effect/incomplete-evidence fences, original-data/repeat-result stability and
typed-limit controls in one selected `-race` invocation (`routingerr` 1.285s,
`dynamic` 1.048s, `orchestrator` 3.003s). Existing manual-recovery, threshold,
duplicate/unsafe-evidence and workflow-claim fence checks also passed in that
invocation. No passing check was repeated; no standing test was added.

## Provider catalog consolidation

The literal catalog covers the following observed signatures. Existing typed
signals and provider-specific rules retain precedence. Classification does not
assert a new account identity, expiry, quota window or circuit scope.

| Signature | Classification |
| --- | --- |
| upstream_endpoint_unavailable | `provider_unavailable` |
| rate_limit_try_later | `rate_limited` |
| usage_limit_reached_reset_in | `quota_limited` |
| account_user_blocked | `account_blocked` |
| ocg_go_usage_limit | `quota_limited` |
| not_found | `model_unavailable` |
| provider_header_timeout | `provider_unavailable` |
| upstream_host_rate_limited | `rate_limited` |
| dp_credit_limit | `quota_limited` |
| or_no_endpoints | `model_unavailable` |
| deepseek_reasoning_content_400 | `resume_corrupted` |
| upstream_idle_timeout_504 | `provider_unavailable` |
| invalid_tools_400 | `agent_runtime_error` |
| ocg_requires_global_region | `provider_not_configured` |
| rate_limit_redacted_reason | `rate_limited` |
| codex_usage_limit | `quota_limited` |
| internal_server_error_500 | `provider_unavailable` |
| ocg_trains_on_data_blocked | `provider_not_configured` |
| stealth_error | `provider_unavailable` |
| nvidia_overloaded_503 | `provider_overloaded` |
| upstream_model_unavailable | `model_unavailable` |
| console_rate_limit | `rate_limited` |
| context_length_400 | `agent_runtime_error` |
| host_capacity_unavailable_429 | `model_capacity` |
| socket_closed | `network_unavailable` |
| nvidia_failed_generate | `provider_unavailable` |
| upstream_invalid_json | `provider_unavailable` |
| unable_to_connect | `network_unavailable` |
| nvidia_not_ready | `provider_unavailable` |
| nvidia_error_code | `provider_unavailable` |
| payment_required | `subscription_required` |
| invalid_request_400 | `agent_runtime_error` |
| rate_limit_brief_wait | `rate_limited` |
| upstream_server_error | `provider_unavailable` |
| reasoning_effort_not_allowed_400 | `agent_runtime_error` |
| backend_overloaded | `provider_overloaded` |
| too_many_images_400 | `agent_runtime_error` |
| provider_empty_response_502 | `provider_unavailable` |
| sse_read_timeout | `provider_unavailable` |
| claude_weekly_limit | `quota_limited` |
| host_503_model_temporarily_unavailable | `provider_unavailable` |
| other:AI_APICallError: Rate limit exceeded: free-models-per-day-high-balance | `rate_limited` |
| other:Internal error: {"code":<n>,"message":"JSON error injected into SSE stream","metadata":{"error_type":"provider_unavailable"}} | `provider_unavailable` |
| free_usage | `quota_limited` |

Existing Go, credit, Codex, Claude, generic-rate, account-block and DeepSeek
reasoning-history signatures are reused. The DeepSeek history diagnostic keeps
`resume_corrupted` and its explicit fresh-session handling. New catalog rules
require a line-opening error envelope or diagnostic type; quoted prose cannot
activate those rules. The previously accepted temporary upstream rate-limit
rule remains unchanged, including the hyphen and whitespace forms.

Only a complete, exact, pre-result request-rejection diagnostic may enter the
existing repeated-unclassified policy. Output, tool effects, missing evidence
or an incomplete identity keep it outside automatic replay. No account/model
selection order, retry count, policy default or reset calendar is changed.

Initial verification used disposable overlays with existing fixtures at `Classify`,
`classifyKanbanFailure` and the dynamic-policy boundary. The overlay is excluded
from automatic test discovery and was not a permanent test suite. The existing
thinking-only empty-turn package is consumed with the upstream lifecycle
diagnostic signature preserved. Its no-answer check is confined to normal
empty completions; reasoning still prevents generic failure replay.

Account-limit learning and account-key attribution are not present at this PR
base. A classification match alone does not prove exhausted-account skipping
or a correct daily/monthly block. Those consumers require their actual source
and account evidence; they must not be simulated by broadening a regex.

## Current-main synchronization and committed coverage

The current-main synchronization keeps both the prompt-context/literal
catalogue sections and the upstream Codex throttling sections. Context matching
remains prompt-send only. The fifteen shared catalogue entries remain separate
from #4826's unlabelled free notice, named daily/raw reasons and general labelled
error_type envelope. Account attribution and automatic recovery policy are
unchanged.

Existing signature_rules_test.go now retains table coverage at Classify for
the shared catalogue and anchored prose boundaries, plus the context phase
boundary. Existing dynamic_unclassified_fallback_test.go retains the actual
classifyKanbanFailure entry-point cases for complete pre-result diagnostics and
ordinary/output/effect/unknown/incomplete evidence. The existing dynamic-policy
admission and upstream Codex/interactive-policy controls are reused.

The integrated candidate passed these focused tests with fts5 and race
detection: routingerr1.912s and orchestrator2.371s. These results cover the
classifier/consumer entry points and existing policy admission; they do not
claim installed-provider or browser execution. The former unrelated Windows
npx-cache fixture limitation remains outside this selected run. Normal commit
hooks and updated-head hosted CI are separate publication results.
