import { test, expect } from "../../fixtures/office-fixture";

test("governance flags persist while unauthenticated admin agent creation bypasses approval", async ({
  officeApi,
  officeSeed,
}) => {
  const { settings } = (await officeApi.getWorkspaceSettings(officeSeed.workspaceId)) as {
    settings: Record<string, unknown>;
  };
  const flags = [
    "require_approval_for_new_agents",
    "require_approval_for_task_completion",
    "require_approval_for_skill_changes",
  ];
  const original = Object.fromEntries(flags.map((flag) => [flag, settings[flag]]));
  expect(typeof settings.require_approval_for_task_completion).toBe("boolean");
  try {
    for (const enabled of [true, false]) {
      await officeApi.updateWorkspaceSettings(
        officeSeed.workspaceId,
        Object.fromEntries(flags.map((flag) => [flag, enabled])),
      );
      const current = (await officeApi.getWorkspaceSettings(officeSeed.workspaceId)) as {
        settings: Record<string, unknown>;
      };
      for (const flag of flags) expect(current.settings[flag]).toBe(enabled);
      // This request has no agent JWT, so it exercises the admin bypass.
      const created = await officeApi.createAgent(officeSeed.workspaceId, {
        name: "Governance Bypass " + enabled,
        role: "worker",
      });
      try {
        expect(created.status).toBe("idle");
      } finally {
        await officeApi.deleteAgent(created.id as string);
      }
    }
  } finally {
    await officeApi.updateWorkspaceSettings(officeSeed.workspaceId, original);
  }
});
