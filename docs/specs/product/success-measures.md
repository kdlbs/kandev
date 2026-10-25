# Product success measures

**Status:** Proposed measurement framework. Target values are not yet approved.

These measures describe whether Kandev helps users complete repository work
safely with agents. They are definitions, not claims that every measure is
currently instrumented. Baselines, cohorts, privacy rules, and target values
must be set before using them as release gates.

## Primary outcome

| Measure | Definition | Desired direction |
| --- | --- | --- |
| Reviewed task completion | Percentage of eligible tasks that reach a human-reviewed outcome such as an accepted change, commit, or pull request. | Increase without reducing review quality. |
| Time to reviewed outcome | Time from task creation to the first human decision that accepts, requests changes to, or rejects the result. | Decrease for comparable work. |
| Rework after review | Share of reviewed tasks requiring a follow-up agent cycle before acceptance. | Decrease, while preserving useful review findings. |

## Activation and usability

| Measure | Definition | Desired direction |
| --- | --- | --- |
| Time to first useful task | Time from installation or workspace creation to the first task with a usable agent response and inspectable change. | Decrease. |
| First-run completion | Percentage of new installations that create a workspace, configure a compatible profile and executor, start an agent, and reach the review surface. | Increase. |
| Recoverable setup failures | Percentage of setup failures that expose an actionable cause and a successful next action without data reset. | Increase. |
| Surface completion parity | Percentage of supported core workflows that remain usable across browser, desktop, headless CLI, and supported MCP paths where parity is promised. | Increase; document intentional limits. |

## Reliability and recovery

| Measure | Definition | Desired direction |
| --- | --- | --- |
| Agent start success | Percentage of start or resume attempts that reach a ready agent session for the selected provider, profile, and executor. | Increase by dependency cohort. |
| Session recovery success | Percentage of interrupted sessions that resume or enter a clear actionable state without losing task context. | Increase. |
| Workspace preservation incidents | Number of incidents where session deletion, restart, cleanup, or executor failure unexpectedly removes task-owned files or Git state. | Zero. |
| Event recovery correctness | Percentage of reconnect, duplicate, stale, or out-of-order event cases that converge to backend state without user repair. | Increase. |
| Cleanup backlog age | Age and count of durable cleanup work that remains unresolved after a task or runtime ends. | Decrease within an approved bound. |

## Safety and trust

| Measure | Definition | Desired direction |
| --- | --- | --- |
| Review coverage | Percentage of tasks that expose changed files and relevant evidence before an external or irreversible action. | Increase. |
| Permission boundary violations | Confirmed cases where an agent, integration, plugin, or surface performs an action outside its configured scope. | Zero. |
| Credential exposure incidents | Confirmed exposure of provider or repository credentials beyond the intended profile and executor boundary. | Zero. |
| Destructive-action clarity | Percentage of destructive flows that show the affected scope and require the intended confirmation. | 100% for supported destructive actions. |

## Product quality

| Measure | Definition | Desired direction |
| --- | --- | --- |
| Dependency-bound success visibility | Percentage of provider, executor, platform, or install-channel failures that identify the dependency and next action. | Increase. |
| Mobile core-flow coverage | Percentage of supported mobile task, session, review, and navigation flows covered by maintained tests and usable without horizontal overflow. | Increase. |
| Localization completeness | Percentage of supported UI copy and locale keys that pass the translation and pseudo-locale checks. | 100% for shipped locales. |
| Upgrade and release integrity | Percentage of release channels and supported artifacts that match the published version, checksum, and runtime contract. | 100%. |

## Measurement rules

- Segment measures by provider, agent CLI, executor, platform, install channel,
  and task size where those dependencies affect the outcome.
- Do not treat agent token volume, task count, or session count as success by
  themselves. They are activity measures, not proof of useful delivery.
- Preserve privacy. Do not collect repository content, prompts, credentials, or
  agent output merely to calculate a product measure.
- Report unsupported or feature-flagged Office behavior separately from the
  supported regular Kanban path.
- Prefer evidence from backend state, review events, tests, and explicit user
  decisions over inferred browser activity.

## Open product questions

- Which three primary measures should become the first product dashboard?
- What baseline period and target values are appropriate for each release
  channel?
- Which measures can be collected locally or anonymously without changing the
  current privacy boundary?
- What is the minimum review evidence required for a task to count as complete?
