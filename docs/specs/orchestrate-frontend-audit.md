# Orchestrate Frontend Hardcoded Logic Audit

## Critical: Backend should provide metadata endpoints

The frontend has hardcoded enums, business logic, and display mappings that should come from the backend.

### Recommended backend endpoints

```
GET /api/v1/orchestrate/meta/statuses
  -> [{id: "backlog", label: "Backlog", order: 0, color: "text-muted-foreground"}]

GET /api/v1/orchestrate/meta/priorities
  -> [{id: "critical", label: "Critical", order: 0, color: "text-red-600"}]

GET /api/v1/orchestrate/meta/roles
  -> [{id: "ceo", label: "CEO", color: "bg-purple-100", defaultPermissions: {...}}]

GET /api/v1/orchestrate/meta/executor-types
  -> [{id: "local_pc", label: "Local (standalone)", description: "Run on host machine"}]

GET /api/v1/orchestrate/meta/skill-source-types
  -> [{id: "inline", label: "Inline", readOnly: false, icon: "code"}]
```

### Findings by severity

#### HIGH -- Business logic duplication

| What | Files affected | Fix |
|------|---------------|-----|
| Status enums (7 statuses) | 10+ files | Backend meta endpoint |
| Priority enums (4 priorities) | 5+ files | Backend meta endpoint |
| Agent roles (4 roles) | 4 files | Backend meta endpoint |
| Skill source types + readOnly logic | 4 files | Return readOnly + reason with each skill |
| Executor types | 3 files | Backend meta endpoint |
| priorityToNumber() conversion | new-issue-dialog.tsx | Backend accepts string priority |
| Default values (role, budget, etc.) | 3 create dialogs | Backend provides defaults in meta endpoint |
| Permission toggles have no server values | settings-content.tsx | Fetch from workspace settings API |

#### MEDIUM -- Color/style mappings

| What | Files affected | Fix |
|------|---------------|-----|
| Status -> color mappings | 6 files | Include variant/color in resource response |
| Agent status dot colors | agent-status-dot.tsx | Backend includes statusVariant |
| Project status badges | project-card.tsx | Backend includes statusVariant |
| Routine run status colors | run-row.tsx | Backend includes statusVariant |
| Inbox item type icons | inbox-item-row.tsx | Backend includes icon/variant |

#### LOW -- Display text

| What | Files affected | Fix |
|------|---------------|-----|
| Read-only reason messages | skill-detail.tsx | Backend returns readOnlyReason |
| Empty state messages | various | OK as frontend concern |
| Page titles | orchestrate-topbar.tsx | OK as frontend concern |
| Color palette options | create-project-dialog.tsx | OK as frontend concern |

### Implementation approach

1. Create `GET /api/v1/orchestrate/meta` endpoint returning ALL metadata in one call
2. Cache in Zustand store on app init (orchestrate layout SSR fetch)
3. Components read from store instead of hardcoded constants
4. Each resource response includes display metadata (variant, label, icon)
5. Skill response includes `readOnly: bool` and `readOnlyReason: string`
