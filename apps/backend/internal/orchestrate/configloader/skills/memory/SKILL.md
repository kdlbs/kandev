# Memory

Read and write persistent memory entries via the kandev API. Memory is organized by layer:
- **operating** -- behavioral guidelines, preferences, communication style
- **knowledge** -- facts about people, projects, systems, decisions

## Write Memory

```bash
curl -s -X PUT http://localhost:${KANDEV_PORT}/api/orchestrate/agents/${KANDEV_AGENT_ID}/memory \
  -H "Authorization: Bearer ${KANDEV_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "layer": "knowledge",
    "key": "people-alice",
    "content": "Alice is the frontend lead. Prefers TypeScript, reviews PRs quickly."
  }'
```

## Read Memory

```bash
# List all memory entries
curl -s http://localhost:${KANDEV_PORT}/api/orchestrate/agents/${KANDEV_AGENT_ID}/memory \
  -H "Authorization: Bearer ${KANDEV_TOKEN}"

# List by layer
curl -s http://localhost:${KANDEV_PORT}/api/orchestrate/agents/${KANDEV_AGENT_ID}/memory?layer=knowledge \
  -H "Authorization: Bearer ${KANDEV_TOKEN}"

# Get specific entry
curl -s http://localhost:${KANDEV_PORT}/api/orchestrate/agents/${KANDEV_AGENT_ID}/memory/knowledge/people-alice \
  -H "Authorization: Bearer ${KANDEV_TOKEN}"
```

## Delete Memory

```bash
curl -s -X DELETE http://localhost:${KANDEV_PORT}/api/orchestrate/agents/${KANDEV_AGENT_ID}/memory/knowledge/people-alice \
  -H "Authorization: Bearer ${KANDEV_TOKEN}"
```

## Guidelines

- Use **operating** for persistent behavioral instructions (communication style, workflow preferences).
- Use **knowledge** for learned facts that should persist across sessions.
- Keep keys descriptive and hyphenated (e.g., `people-alice`, `project-api-migration`).
- Memory entries are markdown files on disk and can be edited by the user directly.
