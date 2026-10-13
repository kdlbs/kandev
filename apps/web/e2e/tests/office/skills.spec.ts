import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/office-fixture";
import { officeTopbarTitle } from "../../helpers/office-topbar";

test.describe("Skills", () => {
  test("canonical skill slug survives create, list and get, and deletion removes it", async ({
    officeApi,
    officeSeed,
  }) => {
    const skill = await officeApi.createSkill(officeSeed.workspaceId, {
      name: "Test Skill",
      slug: "test-skill",
      content: "# Test\n\nThis is a test skill.",
    });
    const id = skill.id as string;
    let deleted = false;
    try {
      expect(skill.slug).toBe("kandev-test-skill");
      const { skills } = (await officeApi.listSkills(officeSeed.workspaceId)) as {
        skills: Array<Record<string, unknown>>;
      };
      expect(skills.some((entry) => entry.slug === "kandev-test-skill")).toBe(true);
      expect(await officeApi.getSkill(id)).toMatchObject({ id, slug: "kandev-test-skill" });
      await officeApi.deleteSkill(id);
      deleted = true;
      const remaining = (await officeApi.listSkills(officeSeed.workspaceId)) as {
        skills: Array<Record<string, unknown>>;
      };
      expect(remaining.skills.some((entry) => entry.id === id)).toBe(false);
    } finally {
      if (!deleted) await officeApi.deleteSkill(id);
    }
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
    // A discovered provider-skill key is a well-formed slug and gets the
    // same write-time canonicalization as any other slug (AC-001.12).
    expect(imported.slug).toBe(`kandev-${key}`);
    expect(imported.source_type).toBe("user_home");
    expect(imported.source_locator).toBe(`${provider}:${key}`);

    const importedId = imported.id as string;
    const supportingFile = await officeApi.getSkillFile(importedId, "guide.md");
    expect(supportingFile.content).toBe("Use the isolated backend HOME only.\n");

    await testPage.goto("/office/workspace/skills");
    await expect(officeTopbarTitle(testPage)).toHaveText(/Skills/i, { timeout: 10_000 });
    await expect(testPage.getByText(name).first()).toBeVisible({ timeout: 10_000 });
  });
});
