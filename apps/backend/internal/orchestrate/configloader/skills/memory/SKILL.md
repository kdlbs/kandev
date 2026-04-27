# Memory

Read and write persistent memory entries via the CLI. Memory is organized by layer:
- **operating** -- behavioral guidelines, preferences, communication style
- **knowledge** -- facts about people, projects, systems, decisions

## Write Memory

```bash
$KANDEV_CLI kandev memory set --layer knowledge --key "people-alice" \
  --content "Alice is the frontend lead. Prefers TypeScript, reviews PRs quickly."
```

## Read Memory

```bash
# List all memory entries
$KANDEV_CLI kandev memory get

# List by layer
$KANDEV_CLI kandev memory get --layer knowledge

# Get specific entry
$KANDEV_CLI kandev memory get --layer knowledge --key "people-alice"
```

## Memory Summary

```bash
$KANDEV_CLI kandev memory summary
```

## Guidelines

- Use **operating** for persistent behavioral instructions (communication style, workflow preferences).
- Use **knowledge** for learned facts that should persist across sessions.
- Keep keys descriptive and hyphenated (e.g., `people-alice`, `project-api-migration`).
- Memory entries are markdown files on disk and can be edited by the user directly.
