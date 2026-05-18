import { test, expect } from "../../fixtures/office-fixture";

test.describe("Routines", () => {
  test("create routine", async ({ officeApi, officeSeed }) => {
    const routine = await officeApi.createRoutine(officeSeed.workspaceId, {
      name: "Daily Check",
      description: "Check system status",
    });
    expect((routine as Record<string, unknown>).name).toBe("Daily Check");
  });

  test("created routine appears in list", async ({ officeApi, officeSeed }) => {
    await officeApi.createRoutine(officeSeed.workspaceId, {
      name: "List Routine",
      description: "Should appear in list",
    });

    const routines = await officeApi.listRoutines(officeSeed.workspaceId);
    const list =
      (routines as { routines?: Record<string, unknown>[] }).routines ??
      (routines as unknown as Record<string, unknown>[]);
    expect(Array.isArray(list) ? list.length : 0).toBeGreaterThan(0);
    expect(
      Array.isArray(list)
        ? list.some((r) => (r as Record<string, unknown>).name === "List Routine")
        : false,
    ).toBe(true);
  });

  test("list routines initially returns array", async ({ officeApi, officeSeed }) => {
    const routines = await officeApi.listRoutines(officeSeed.workspaceId);
    const list =
      (routines as { routines?: Record<string, unknown>[] }).routines ??
      (routines as unknown as Record<string, unknown>[]);
    expect(Array.isArray(list)).toBe(true);
  });

  test("routines page renders", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/routines");
    await expect(testPage.getByRole("heading", { name: /Routines/i }).first()).toBeVisible({
      timeout: 10_000,
    });
  });
});
