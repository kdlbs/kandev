export const CODE_HOST_REVIEW_KEY = "northstar-labs/relay#42";

export const CODE_HOST_PULL_REQUEST = {
  id: "42",
  review_key: CODE_HOST_REVIEW_KEY,
  number: 42,
  title: "Add audit log export with retention controls",
  description: "Adds scoped exports, a 30-day retention policy, and audit events.",
  url: "https://bitbucket.org/northstar-labs/relay/pull-requests/42",
  repository_id: "northstar-labs/relay",
  repository_name: "relay",
  state: "OPEN",
  author_display_name: "Maya Chen",
  created_at: "2026-08-07T09:30:00Z",
  updated_at: "2026-08-10T15:30:00Z",
  source_branch: "feature/audit-export",
  destination_branch: "main",
  head_commit: "c1c53106bebf",
  capabilities: ["approve", "decline", "comments", "thread_replies"],
  files: [
    { path: "internal/audit/export.go", status: "modified", additions: 84, deletions: 12 },
    { path: "internal/audit/export_test.go", status: "added", additions: 96, deletions: 0 },
  ],
  commits: [
    {
      id: "c1c53106bebf",
      message: "feat(audit): add scoped export retention",
      author: "Ari Almeida",
    },
  ],
  participants: [
    { id: "maya", name: "Maya Chen", role: "REVIEWER", approved: true },
    { id: "noah", name: "Noah Williams", role: "REVIEWER", approved: false },
  ],
  statuses: [
    { key: "build", name: "Build and test", state: "SUCCESSFUL", target: "feature/audit-export" },
    { key: "security", name: "Security scan", state: "SUCCESSFUL", target: "feature/audit-export" },
  ],
  threads: [
    {
      id: "comment-7",
      file: "internal/audit/export.go",
      comments: [
        {
          id: "comment-7",
          author: "Maya Chen",
          body: "Can we cap each export batch?",
          created_at: "2026-08-10T12:00:00Z",
        },
        {
          id: "comment-8",
          parent_id: "comment-7",
          author: "Ari Almeida",
          body: "Capped at 5,000 rows.",
          created_at: "2026-08-10T13:10:00Z",
        },
      ],
    },
  ],
} as const;
