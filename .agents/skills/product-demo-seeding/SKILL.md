---
name: product-demo-seeding
description: Seed coherent, disposable Kandev demo scenarios for screenshots, product films, landing-page media, and reproducible UI captures. Use when media needs believable tasks, workflows, agents, executors, integrations, plans, sessions, diffs, reviews, or native-mobile states; invoke before product-video-capture and never use a developer's main instance or data.
---

# Product Demo Seeding

Build a truthful product state before recording pixels. Treat narrative, data, and UI route as one artifact.

## Current-Main Source Gate

Product capture represents current Kandev, so old checkouts and old builds are invalid even when their scripts still run.

1. Fetch `origin/main` immediately before capture setup.
2. Create a clean detached capture worktree from `origin/main`; do not reuse the task worktree or a landing branch.
3. Record both SHAs and prove `HEAD` equals `origin/main`. Require an empty `git status --porcelain` before building and before capture.
4. Install dependencies and build frontend/backend from that worktree. Verify it contains `scripts/dev-isolated` and `apps/web/e2e/`.
5. Stage specs, raw masters, configs, proofs, logs, and delivery candidates under a fresh artifact root outside production assets.

Reject stale UI, selector, script, or capture choreography until current source is built and the complete route passes rehearsal. Never substitute an old server, cached bundle, or last month's working selectors for this gate.

## Workflow

1. Read `/e2e` and the relevant existing specs/page objects under `apps/web/e2e/`.
2. Write a one-sentence story: user goal, visible action, visible result.
3. Identify separate desktop and native-mobile routes. Mobile must use native mobile surfaces, not a desktop crop.
4. Explore manually with `scripts/dev-isolated --web` when needed. For reproducible capture, use the worker-scoped E2E fixture and `ApiClient`.
5. Create a fresh fictional repository, workspace, workflow, tasks, sessions, and provider state through supported E2E/API methods.
6. Seed only enough state to make the story legible. Dense believable data beats empty fixtures; excessive data hides the action.
7. Open the intended UI and verify every visible label, control, and transition before recording.
8. Rehearse without a recorder, discard rehearsal mutations, then reseed an identical pristine baseline for each take.
9. Hand the scenario, routes, selectors, semantic target bounds, complete intentional pointer journeys, provenance, and cleanup command to `/product-video-capture`.

## Safety Contract

- Allocate a fresh temp `HOME`, `KANDEV_HOME_DIR`, database, repository/worktree root, unique ports, unique display, unique browser profile, and artifact root. Names and ownership must identify one take.
- Use deterministic mock GitHub and Jira providers plus a controlled agent. Use fixed IDs and timestamps, authors, titles, bodies, states, checks, and ordering.
- No credentials or network access to real provider services is permitted. Unset provider tokens and fail closed if mock routing cannot be proven in backend logs.
- Never copy the developer's database for marketing capture. `scripts/dev-isolated --copy-db` is outside this workflow.
- Never query, mutate, stop, or reuse the developer's instance, DB, or data.
- Keep temporary capture specs or harness code under `CAPTURE_ROOT`, outside the source worktree. If the runner requires an in-worktree file, write only its exact path to a take-owned excludes file under `CAPTURE_ROOT` and pass it through command-local `core.excludesFile` configuration inherited by the capture process. Prove the clean-worktree status gate still returns an empty `git status --porcelain`, stage a source copy, then remove the file and exclusion during teardown. Never mutate the shared `.git/info/exclude`; linked worktrees and sibling agents may depend on it.
- Stop if isolation cannot be proven from process args, ports, paths, and logs.

Read [isolation-and-seeding.md](references/isolation-and-seeding.md) before starting an instance.

## Truthfulness Contract

- Seed real records and drive real controls. Do not fabricate menu items, executor families, integrations, checks, or agent support in the DOM.
- Prefer coherent API-seeded labels over capture-time text replacement. If fixture sanitation is unavoidable, document exact substitutions; never add controls, hide product bugs, or change behavior.
- Use current product capabilities. Inspect selectors and API helpers again instead of copying an old capture spec blindly.
- If a responsive surface is broken, report the product bug and choose another truthful native route only when it demonstrates the same capability.
- Keep local paths, test directives, generic mock responses, fixture names, and host identity out of visible frames.
- Give capture-only mock executors a logical command label such as `mock-agent`; never expose the temporary executable's absolute host path in a visible command, profile, or session label.

## Per-Take State Contract

Rehearsal changes product state: integration flows create tasks. A successful rehearsal therefore cannot share recording state.

- Start from a fresh database for every recorded take. An equivalent reset is acceptable only when it deletes all scenario records and reseeds one canonical baseline with a verified seed hash.
- Use a rehearsal reset or discard its entire temp root before RECORD. Restart backend/browser ownership as needed so cached client state cannot survive.
- Before each take, assert exact workspace/workflow/task/provider counts, unique IDs, unique visible task titles, and expected ordering. Reject duplicate or accumulated tasks, fixtures, sessions, or provider-created records.
- Give desktop GitHub, mobile GitHub, desktop Jira, and mobile Jira independent state ownership. One failed or repeated take must not contaminate another.
- Never clean visible state with DOM deletion, CSS hiding, or capture-time text replacement. Fix the seed or start fresh.

## Story Selection

Use [story-recipes.md](references/story-recipes.md) for Plan, Coordinate, Prepare, Run, Review, integrations, editor/terminal, and mobile recipes. Recipes are patterns, not frozen scripts.

## Acceptance Gate

Before capture, verify:

- Story has a clear initial state, action, and result. Treat 7-11 seconds as a target, never a reason to omit an honest route step, rush readability, or shorten the settled loop.
- Desktop and mobile each have a native script and safe composition.
- Visible data forms one fictional project narrative.
- No production endpoint, credential, local path, fixture copy, or unsupported control is visible.
- Current build and all story-critical routes passed a recorder-free rehearsal; rehearsal state was reset or discarded.
- Exact baseline record counts prove no duplicate fixture accumulation before RECORD.
- Seed teardown removes temporary profiles, specs, processes, ports, database, and repository.

Report seed name, story, source SHA, seed hash, fixed provider fixture version, separate desktop/native-mobile routes, process args, ports, display, browser profile, database/temp/artifact roots, mock-routing proof, any sanitation, and teardown result. This provenance must map every delivered take to its isolated baseline and current-main source.
