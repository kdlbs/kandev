# Impact authoring

Read this reference before inspecting a PR and writing its walkthrough JSON.
All content describes the comparison base and PR head, not a guessed release history.

## Concise content

- Lead with who encounters the problem, the trigger, and the previous consequence.
- Explain the outcome in user terms. Avoid claims such as "improves reliability" without a concrete result.
- Keep table cells to one short sentence or a signature plus a short explanation.
- Use one row per distinct UX outcome, API, tool in a context, or database object.
- Group repeated mechanical schema edits only when their constraints and upgrade effects match.
- Do not cap interface or migration inventories at the code canvas limit. Account for every changed public contract.
- Use diagrams only to explain relationships that the tables cannot show clearly.

## JSON shape

Add `why.audience` and `why.outcome` beside the existing `problem` and `what` fields.
Add an `impact` object with these five keys: `breaking`, `ux`, `plugins`, `mcp`, and `database`.
Each category has `status`, `items`, and an optional `note`:

| Status | Items | Meaning |
| --- | --- | --- |
| `changed` | At least one row | Evidence establishes a change. |
| `none` | Empty array | Relevant code and contracts show no change. |
| `unknown` | Empty array | Evidence is unavailable. A `note` must explain the limit. |

If known changes coexist with evidence gaps, use `changed` and explain the remaining uncertainty in `note`.
Never use `none` merely because the PR description omits the topic.
All row fields in the next table are required, non-empty strings.
Every row also needs `file`, a repository-relative evidence path. The renderer links it to the PR diff.
Use a changed file that establishes the behavior. Mention supporting unchanged contracts in the cell when necessary.

| Category | Row fields | Evidence and content |
| --- | --- | --- |
| `breaking` | `audience`, `before`, `after`, `action` | Identify affected users and the trigger. State previous behavior, new failure or incompatibility, and migration action. |
| `ux` | `surface`, `before`, `after` | Name the entry point, such as Settings or Create task. Explain visible controls, navigation, defaults, messages, and outcomes. Include phone differences when present. |
| `plugins` | `name`, `change`, `contract`, `compatibility` | List exact public API names. Identify Host UI, frontend types, SDK, wire protocol, manifest, or package contract. Show changed signatures or behavior and required plugin updates. |
| `mcp` | `name`, `context`, `change`, `contract`, `compatibility` | Use canonical tool names. State inputs, outputs, permissions, defaults, or behavior that changes. Identify caller access and registration scope. |
| `database` | `migration`, `object`, `change`, `upgrade` | Name the migration ID/path and database. Identify tables, columns, indexes, types, nullability, defaults, constraints, or removals. Explain backfills, existing rows, and rollback limits. |

For plugins and MCP, `change` is `added`, `changed`, or `removed`.
Use `compatibility` to state compatible, breaking, or unverified, followed by the reason and caller action.
Repeat breaking contract changes in `breaking` as concise user consequences. Do not hide them in technical tables.
For schema changes without a migration, explicitly state "No migration" and explain the effect on existing installations.
Do not claim safe rollback, data preservation, or compatibility without evidence.

## Inspection checklist

### Visible UX

Trace rendered controls and their routes, dialogs, handlers, and error states.
A renamed React component or moved hook with identical behavior is `none`.
A new settings page, task-dialog option, validation error, or changed default is a UX change.
Describe the experience, not component names or CSS internals.

### Kandev plugin interfaces

Inspect public exports and their consumers, not only files with "plugin" in the name.
Relevant boundaries include `apps/web/lib/plugins/types.ts`, `host-api.ts`, and the plugin API guide.
Backend boundaries include `apps/backend/pkg/pluginsdk`, the plugin proto, and manifest/package validation.
An internal host refactor without a public contract change is `none`.
Additions can break exhaustive consumers or version negotiation. Assess the actual contract.

### MCP tools and growth

Trace registration and availability gates as well as handlers and schemas.
Identify external MCP, task MCP, Office task MCP, configuration MCP, or another actual server context.
Include role restrictions and flags in the context or contract. Do not assume every tool reaches every agent.
Use a consistent context label for all tools on the same surface.

The renderer derives added, changed, removed, and net counts for each context from the rows.
These are counts of exposed tool names, not handlers or schema fields.
A renamed tool is one removal and one addition. A tool moved between contexts follows the same rule.
A tool exposed in two contexts needs two rows. Do not count either row twice in its context.
A schema-only edit is `changed`, with zero net growth.
Explain why a new tool exists and whether it extends or replaces an existing tool in `contract`.
If the PR does not establish that rationale, say it is not established. Do not invent a justification.

### Database changes

Inspect migration registration, SQL, repository initialization, and upgrade tests.
Separate persisted database changes from in-memory structs and API payload fields.
Record destructive operations, new required columns, defaults for existing rows, and migration order when relevant.

### Breaking behavior

Compare successful base behavior with head behavior for existing users and callers.
Include stricter validation, removed fallback, changed permissions, incompatible defaults, and new required configuration.
Do not limit this section to removed APIs or major version changes.

Illustrative row, only when the inspected PR proves this change:

```json
{
  "audience": "Users whose executor profile model differs from Settings",
  "before": "Task startup selects a fallback model automatically.",
  "after": "Task startup stops with a model mismatch error.",
  "action": "Select a matching model before starting the task.",
  "file": "path/to/changed/model-validation.go"
}
```

This example is not evidence about the current PR.
If compatibility cannot be established, report the uncertainty instead of inventing a breaking change or declaring compatibility.

## Validation

Verify that impact appears before architecture and code.
Check that unchanged categories have no empty detail tables.
Read the page at desktop and phone widths. Phone tables must show labeled cards with all fields and source links.
Verify breaking alerts in both themes and MCP counts against the registration diff.

After renderer or shell changes, run these commands from the repository root:

```bash
python3 -m unittest discover -s .agents/skills/pr-walkthrough/references -p test_build.py
python3 .agents/skills/pr-walkthrough/scripts/pr-walkthrough-render.test.py
node .agents/skills/pr-walkthrough/references/test_browser.cjs
```

The browser check uses workspace Playwright and an installed Chromium browser.
It renders synthetic impact rows and verifies phone cards, desktop tables, both themes, source links, and section navigation.
