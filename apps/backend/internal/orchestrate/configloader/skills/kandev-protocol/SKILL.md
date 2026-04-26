# Kandev Protocol

You are an agent managed by kandev. Follow these interaction rules:

## Heartbeat

Send a heartbeat every 60 seconds while working:
```bash
curl -s -X POST http://localhost:${KANDEV_PORT}/api/orchestrate/heartbeat \
  -H "Authorization: Bearer ${KANDEV_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"agent_instance_id": "'${KANDEV_AGENT_ID}'"}'
```

## Task Completion

When you finish a task, report completion:
```bash
curl -s -X POST http://localhost:${KANDEV_PORT}/api/orchestrate/tasks/${KANDEV_TASK_ID}/complete \
  -H "Authorization: Bearer ${KANDEV_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"summary": "description of what was done"}'
```

## Comments

Post comments to your task for async communication with other agents or the user:
```bash
curl -s -X POST http://localhost:${KANDEV_PORT}/api/orchestrate/tasks/${KANDEV_TASK_ID}/comments \
  -H "Authorization: Bearer ${KANDEV_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{"body": "your message here"}'
```

## Environment Variables

These are injected into your session automatically:
- `KANDEV_PORT` -- backend API port
- `KANDEV_TOKEN` -- JWT for API authentication
- `KANDEV_AGENT_ID` -- your agent instance ID
- `KANDEV_TASK_ID` -- the current task ID
- `KANDEV_WORKSPACE_ID` -- the workspace you belong to
