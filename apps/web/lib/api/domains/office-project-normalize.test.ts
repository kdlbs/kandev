import { describe, expect, it } from "vitest";
import { normalizeProject } from "./office-project-normalize";

describe("normalizeProject", () => {
  it("maps the backend project wire shape to workspace-owned web data", () => {
    expect(
      normalizeProject({
        id: "project-1",
        workspace_id: "workspace-1",
        name: "Platform",
        description: "Shared services",
        status: "active",
        lead_agent_profile_id: "agent-1",
        budget_cents: 2500,
        repositories: ["github.com/acme/platform"],
        executor_config: '{"type":"local_docker"}',
        task_counts: { total: 3, in_progress: 1, done: 1, blocked: 1 },
        created_at: "2026-01-01T00:00:00Z",
        updated_at: "2026-01-02T00:00:00Z",
      }),
    ).toEqual({
      id: "project-1",
      workspaceId: "workspace-1",
      name: "Platform",
      description: "Shared services",
      status: "active",
      leadAgentProfileId: "agent-1",
      color: "",
      budgetCents: 2500,
      repositories: ["github.com/acme/platform"],
      executorConfig: { type: "local_docker" },
      taskCounts: { total: 3, in_progress: 1, done: 1, blocked: 1 },
      createdAt: "2026-01-01T00:00:00Z",
      updatedAt: "2026-01-02T00:00:00Z",
    });
  });
});
