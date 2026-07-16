---
name: product-demo-seeding
description: Seed coherent, disposable Kandev demo scenarios for screenshots, product films, landing-page media, and reproducible UI captures. Use whenever media needs believable tasks, workflows, agents, executors, integrations, plans, sessions, diffs, reviews, or native mobile states; invoke before product-video-capture and never use a developer's main instance or data.
---

# Product Demo Seeding

Build a truthful product state before recording pixels. Treat narrative, data, and UI route as one artifact.

## Prerequisites

Use a Kandev checkout with its web E2E fixtures, Playwright dependencies, pnpm toolchain, Git, and a disposable local backend available. Verify the checkout contains `scripts/dev-isolated` and `apps/web/e2e/` before creating seed state.

## Workflow

1. Read `/e2e` and the relevant existing specs/page objects under `apps/web/e2e/`.
2. Write a one-sentence story: user goal, visible action, visible result.
3. Identify separate desktop and mobile routes. Mobile must use native mobile surfaces, not a desktop crop.
4. Explore manually with `scripts/dev-isolated --web` when needed. For reproducible capture, use the worker-scoped E2E fixture and `ApiClient`.
5. Create a fresh bogus repository, workspace, workflow, tasks, sessions, and provider state through supported E2E/API methods.
6. Seed only enough state to make the story legible. Dense believable data beats empty fixtures; excessive data hides the action.
7. Open the intended UI and verify every visible label, control, and transition before recording.
8. Hand the scenario, route, selectors, semantic target bounds, and cleanup command to `/product-video-capture`.

## Safety Contract

- Use a fresh temp `HOME`, `KANDEV_HOME_DIR`, SQLite path, repository, worktree root, and non-production ports.
- Use mock external providers and a mock agent. Never load credentials or contact real GitHub/Jira/Linear/Sentry/Slack services.
- Never copy the developer's database for marketing capture. `scripts/dev-isolated --copy-db` is outside this workflow.
- Never query, mutate, stop, or reuse the main Kandev instance.
- Keep capture specs temporary or in a reviewed source bundle. Remove temporary copies after the run.
- Stop if isolation cannot be proven from process args, ports, paths, and logs.

Read [isolation-and-seeding.md](references/isolation-and-seeding.md) before starting an instance.

## Truthfulness Contract

- Seed real records and drive real controls. Do not fabricate menu items, executor families, integrations, checks, or agent support in the DOM.
- Prefer coherent API-seeded labels over capture-time text replacement. If fixture sanitation is unavoidable, document exact substitutions; never add controls, hide product bugs, or change behavior.
- Use current product capabilities. Inspect selectors and API helpers again instead of copying an old capture spec blindly.
- If a responsive surface is broken, report the product bug and choose another truthful native route only when it demonstrates the same capability.
- Keep local paths, test directives, generic mock responses, fixture names, and host identity out of visible frames.

## Story Selection

Use [story-recipes.md](references/story-recipes.md) for Plan, Coordinate, Prepare, Run, Review, integrations, editor/terminal, and mobile recipes. Recipes are patterns, not frozen scripts.

## Acceptance Gate

Before capture, verify:

- Story has a clear initial state, action, and result in 7-11 seconds.
- Desktop and mobile each have a native script and safe composition.
- Visible data forms one fictional project narrative.
- No production endpoint, credential, local path, fixture copy, or unsupported control is visible.
- Seed teardown removes temporary profiles, specs, processes, ports, database, and repository.

Report seed name, product story, ports, temp roots, mock providers, real UI route, any sanitation, and teardown result.
