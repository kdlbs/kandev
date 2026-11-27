import { test, expect } from "../../fixtures/office-fixture";

test("config export, filesystem diff and import routes round-trip the workspace", async ({
  officeApi,
  officeSeed,
}) => {
  const exported = (await officeApi.exportConfig(officeSeed.workspaceId)) as {
    bundle: { settings: { name: string } };
  };
  expect(exported.bundle.settings.name).toBe(officeSeed.workspaceId);
  expect(await officeApi.applyOutgoingSync(officeSeed.workspaceId)).toEqual({ status: "ok" });
  const diff = await officeApi.getIncomingDiff(officeSeed.workspaceId);
  expect(diff).toHaveProperty("diff");
  const imported = await officeApi.applyIncomingSync(officeSeed.workspaceId);
  expect(imported).toHaveProperty("result");
});
