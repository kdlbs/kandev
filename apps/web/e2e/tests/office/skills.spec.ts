import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/office-fixture";

test.describe("Skills", () => {
  test("create and list skill", async ({ officeApi, officeSeed }) => {
    const skill = await officeApi.createSkill(officeSeed.workspaceId, {
      name: "Test Skill",
      slug: "test-skill",
      content: "# Test\n\nThis is a test skill.",
    });
    expect((skill as Record<string, unknown>).slug).toBe("test-skill");

    const skills = await officeApi.listSkills(officeSeed.workspaceId);
    const list =
      (skills as { skills?: Record<string, unknown>[] }).skills ??
      (skills as unknown as Record<string, unknown>[]);
    expect(
      Array.isArray(list)
        ? list.some((s) => (s as Record<string, unknown>).slug === "test-skill")
        : false,
    ).toBe(true);
  });

  test("get skill by id", async ({ officeApi, officeSeed }) => {
    const created = await officeApi.createSkill(officeSeed.workspaceId, {
      name: "Get Skill",
      slug: "get-skill",
      content: "# Get\n\nFetch by ID.",
    });
    const id = (created as Record<string, unknown>).id as string;
    const fetched = await officeApi.getSkill(id);
    expect((fetched as Record<string, unknown>).id).toBe(id);
    expect((fetched as Record<string, unknown>).slug).toBe("get-skill");
  });

  test("delete skill removes it from list", async ({ officeApi, officeSeed }) => {
    const created = await officeApi.createSkill(officeSeed.workspaceId, {
      name: "Delete Skill",
      slug: "delete-skill",
      content: "# Delete\n\nWill be removed.",
    });
    const id = (created as Record<string, unknown>).id as string;

    await officeApi.deleteSkill(id);

    const skills = await officeApi.listSkills(officeSeed.workspaceId);
    const list =
      (skills as { skills?: Record<string, unknown>[] }).skills ??
      (skills as unknown as Record<string, unknown>[]);
    expect(
      Array.isArray(list) ? list.some((s) => (s as Record<string, unknown>).id === id) : false,
    ).toBe(false);
  });

  test("skills page renders", async ({ testPage, officeSeed: _ }) => {
    await testPage.goto("/office/workspace/skills");
    await expect(testPage.getByRole("heading", { name: /Skills/i }).first()).toBeVisible({
      timeout: 10_000,
    });
  });

  test("discovers and imports provider skill from isolated user home", async ({
    backend,
    officeApi,
    officeSeed,
    testPage,
  }) => {
    const key = `user-home-${Date.now()}`;
    const name = `User Home Skill ${Date.now()}`;
    const provider = "mock-agent";
    const skillDir = path.join(backend.tmpDir, ".mock-agent", "skills", key);
    fs.mkdirSync(skillDir, { recursive: true });
    fs.writeFileSync(
      path.join(skillDir, "SKILL.md"),
      `---\nname: ${name}\ndescription: Imported from isolated E2E home\n---\n# ${name}\n`,
    );
    fs.writeFileSync(path.join(skillDir, "guide.md"), "Use the isolated backend HOME only.\n");

    const discovered = await officeApi.discoverUserHomeSkills(officeSeed.workspaceId, provider);
    const discoveredSkills = (discovered.skills ?? []) as Record<string, unknown>[];
    expect(discoveredSkills.some((skill) => skill.key === key && skill.name === name)).toBe(true);

    const imported = await officeApi.importUserHomeSkill(officeSeed.workspaceId, provider, key);
    expect(imported.name).toBe(name);
    expect(imported.slug).toBe(key);
    expect(imported.source_type).toBe("user_home");
    expect(imported.source_locator).toBe(`${provider}:${key}`);

    const importedId = imported.id as string;
    const supportingFile = await officeApi.getSkillFile(importedId, "guide.md");
    expect(supportingFile.content).toBe("Use the isolated backend HOME only.\n");

    await testPage.goto("/office/workspace/skills");
    await expect(testPage.getByText(name).first()).toBeVisible({ timeout: 10_000 });
  });
});
