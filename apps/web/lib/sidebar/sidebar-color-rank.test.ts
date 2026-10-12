import { describe, expect, it } from "vitest";
import fixture from "../../../backend/internal/task/repository/testdata/sidebar-color-conformance.json";
import type { TaskSwitcherItem } from "@/components/task/task-switcher-types";
import type { SidebarTaskColorAutomation } from "@/lib/task-color-automation-settings";
import type { TaskColor } from "@/lib/task-colors";
import { effectiveSidebarColorToken } from "./sidebar-color-rank";
import { repositoryIdentityForSavedRepository } from "./repository-rule-identity";

describe("sidebar color conformance", () => {
  for (const scenario of fixture.cases) {
    it(scenario.name, () => {
      const item: TaskSwitcherItem = {
        id: "target",
        title: "Target",
        workspaceId: scenario.workspace_id,
        workflowId: scenario.task.workflow_id,
        workflowStepId: scenario.task.workflow_step_id,
        workflowStepColor: scenario.steps?.find(
          (step) => step.id === scenario.task.workflow_step_id,
        )?.color,
        state: (scenario.task.state ?? "TODO") as TaskSwitcherItem["state"],
        priority: scenario.task.priority as TaskSwitcherItem["priority"],
        origin: scenario.task.origin,
        primaryExecutorProfileId: (scenario.task.primary_session_profile_id ??
          scenario.task.metadata?.executor_profile_id) as string | undefined,
        repositoryRuleIdentities: scenario.repository
          ? [
              repositoryIdentityForSavedRepository({
                id: scenario.repository.id,
                workspace_id: scenario.workspace_id,
                provider: scenario.repository.provider ?? "",
                provider_repo_id: scenario.repository.provider_repo_id ?? "",
                provider_host: scenario.repository.provider_host,
                provider_scope: scenario.repository.provider_scope,
                provider_owner: scenario.repository.provider_owner,
                remote_url: scenario.repository.remote_url,
                local_path: scenario.repository.local_path ?? "",
              }),
            ]
          : [],
      };
      const token = effectiveSidebarColorToken(item, {
        automation: scenario.automation as SidebarTaskColorAutomation,
        manualColors: { target: (scenario.manual_color ?? null) as TaskColor | null },
      });

      expect(token).toBe(scenario.expected_token);
      expect(token === scenario.preferred_color).toBe(scenario.expected_match);
    });
  }
});
