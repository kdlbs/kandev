---
name: kandev-projects
description: List and create Office projects, then place new tasks in the correct project when organizing workspace work by repository.
kandev:
  system: true
  version: "0.42.0"
  default_for_roles: [ceo]
---

# Projects

Use project IDs returned by the CLI. Project access is scoped to the current
Office workspace, and project creation is permission checked.

## Inspect projects

```bash
$KANDEV_CLI kandev projects list
```

## Create a project

`--name` is required. Repeat `--repository` for each repository URL or local
path the project owns.

```bash
$KANDEV_CLI kandev projects create \
  --name "Payments" \
  --description "Payment services and checkout" \
  --repository "https://github.com/acme/payments" \
  --repository "/workspace/checkout"
```

Optional project fields are `--lead-agent-profile-id`, `--color`,
`--budget-cents`, and `--executor-config`.

## Create work in a project

Pass the returned project ID when creating a task:

```bash
$KANDEV_CLI kandev task create \
  --title "Add payment retry policy" \
  --project "$PROJECT_ID" \
  --assignee "$AGENT_ID"
```

Do not create a duplicate project when an existing project already owns the
repository. Office runs do not administer workspaces.
