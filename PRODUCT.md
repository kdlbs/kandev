# Product

## Users

Kandev is for developers and engineering teams using AI agents for code work, reviews, workflows, and repository automation. Users arrive with active engineering context: tasks in flight, sessions running, worktrees diverging, repositories changing, integrations reporting state, and follow-up decisions waiting on them.

## Product Purpose

Kandev coordinates agentic development work across tasks, sessions, worktrees, repositories, and integrations. The product should help users understand what is happening, choose the next command, and move work through a workflow without losing trust in the underlying developer environment.

Success means users can quickly answer:

- What task, repository, branch, session, and workflow step am I in?
- What changed, what is running, and what needs attention?
- Which commands are available now, and which are secondary or conditional?
- How does this agent-driven work connect back to GitHub, Jira, editors, terminals, and local or remote execution?

## Brand Personality

Focused, technical, composed.

The interface should feel like a reliable developer workbench: direct, legible, and confident under pressure. It should avoid performative futurism and keep the user oriented in real engineering state.

## Anti-references

- Generic SaaS dashboards with decorative metrics and inflated spacing.
- Decorative AI gradients, glowing blobs, and "magic" visual language.
- Cluttered IDE chrome that competes with the actual work.
- Novelty terminal cosplay, fake hacker styling, and retro effects used as decoration.

## Design Principles

### Orientation before controls

The user should first understand their current scope: workspace, repository, task, session, branch, workflow step, and execution state. Controls follow orientation instead of crowding it.

### Command-first secondary actions

Primary actions should be obvious and state-aware. Secondary actions should be available through predictable command surfaces, menus, panel headers, or contextual toolbars rather than scattered across global chrome.

### Density without clutter

Kandev is a professional tool with real state density. Use compact controls and information-rich rows, but preserve grouping, alignment, and hierarchy so the surface can be scanned without fatigue.

### State visible but calm

Running agents, remote execution, git status, reviews, tunnels, health, and workflow progress should be visible without becoming alarmist. Color and motion indicate meaning, not decoration.

### Familiar affordances preserve developer trust

Use recognizable developer patterns for navigation, git, terminals, panels, editors, filters, and settings. Novel interaction should be reserved for places where the workflow genuinely needs it.

## Accessibility & Inclusion

Target WCAG AA. Interaction must remain reachable by keyboard, with visible focus states and screen-reader labels for icon-only controls. Status must not depend on color alone; pair color with labels, icons, tooltips, or text. Support reduced motion by avoiding nonessential animation and keeping state transitions short.
