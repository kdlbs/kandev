import { execSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { makeGitEnv } from "../../helpers/git-helper";

test.describe("Mobile workspace repository sets", () => {
  test("scrolls a long branch list by touch without dismissing the editor", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    const dir = path.join(backend.tmpDir, "repos", "mobile-set-scroll");
    fs.mkdirSync(dir, { recursive: true });
    const gitEnv = makeGitEnv(backend.tmpDir);
    execSync('git init -b main && git commit --allow-empty -m "init"', { cwd: dir, env: gitEnv });
    for (let index = 0; index < 40; index++) {
      execSync(`git branch scroll-test-${index}`, { cwd: dir, env: gitEnv });
    }
    const repository = await apiClient.createRepository(seedData.workspaceId, dir, "main", {
      name: "Mobile branch scrolling",
    });
    const set = await apiClient.createRepositorySet(seedData.workspaceId, "Touch scroll", [
      repository.id,
    ]);
    try {
      await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
      await testPage.getByTestId(`repository-set-edit-${set.id}`).tap();
      await testPage.getByTestId(`repository-set-base-${repository.id}`).tap();
      const dropdown = testPage.getByTestId(`repository-set-base-dropdown-${repository.id}`);
      const list = dropdown.getByRole("listbox");
      await expect(list.getByRole("option", { name: /scroll-test-39/ })).toBeAttached();
      const box = await list.boundingBox();
      expect(box).not.toBeNull();
      const cdp = await testPage.context().newCDPSession(testPage);
      try {
        const x = box!.x + box!.width / 2;
        const y = box!.y + box!.height - 20;
        await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x, y }] });
        for (let step = 1; step <= 6; step++) {
          await cdp.send("Input.dispatchTouchEvent", {
            type: "touchMove",
            touchPoints: [{ x, y: y - step * 30 }],
          });
        }
        await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
        await expect.poll(() => list.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
      } finally {
        await cdp.detach();
      }
      await expect(testPage.getByTestId("repository-set-editor-surface")).toBeVisible();
      await expect(dropdown).toBeVisible();
      await assertNoDocumentHorizontalOverflow(testPage, "mobile branch scroll");
      await prCapture.screenshot("mobile-repository-set-branch-scroll", {
        caption:
          "Touch scrolling moves the branch list without dismissing the repository set editor.",
      });
      await dropdown.getByPlaceholder("Search branches...").fill("scroll-test-39");
      await dropdown.getByRole("option", { name: /scroll-test-39/ }).tap();
      await expect(testPage.getByTestId(`repository-set-base-${repository.id}`)).toContainText(
        "scroll-test-39",
      );
    } finally {
      await apiClient.deleteRepositorySet(set.id);
      await apiClient.rawRequest("DELETE", `/api/v1/repositories/${repository.id}`);
    }
  });

  test("opens the inline editor as a contained full-height drawer", async ({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const setName = `Mobile editor set ${Date.now()}`;
    const created = await apiClient.createRepositorySet(seedData.workspaceId, setName, [
      seedData.repositoryId,
    ]);

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
    await testPage.getByTestId(`repository-set-edit-${created.id}`).tap();

    const surface = testPage.getByTestId("repository-set-editor-surface");
    await expect(surface).toBeVisible();
    await expect(surface).toHaveClass(/h-\[100dvh\]/);
    await expect.poll(async () => (await surface.boundingBox())?.height).toBe(844);
    await expect(testPage.getByTestId("repository-set-editor-form")).toHaveClass(
      /min-h-0.*overflow-y-auto/,
    );
    const membersHint = testPage.getByText(
      "Add repositories in task order. Base branches are optional.",
    );
    const addRepository = testPage.getByTestId("repository-set-add-repository");
    const [membersHintBox, addRepositoryBox] = await Promise.all([
      membersHint.boundingBox(),
      addRepository.boundingBox(),
    ]);
    expect(membersHintBox).not.toBeNull();
    expect(addRepositoryBox).not.toBeNull();
    expect(addRepositoryBox!.y).toBeGreaterThan(membersHintBox!.y + membersHintBox!.height);
    expect(addRepositoryBox!.width).toBeCloseTo(membersHintBox!.width, 0);
    await testPage.getByTestId(`repository-set-remove-${seedData.repositoryId}`).tap();
    await addRepository.tap();
    await testPage.getByRole("option", { name: /E2E Repo/ }).tap();
    await expect(
      testPage.getByTestId(`repository-set-base-${seedData.repositoryId}`),
    ).toBeVisible();
    await prCapture.screenshot("mobile-repository-set-editor", {
      caption:
        "The mobile repository set editor uses a full-height drawer with a fixed action bar.",
    });

    for (const control of [
      testPage.getByTestId("repository-set-editor-save"),
      testPage.getByTestId("repository-set-editor-cancel"),
      testPage.getByTestId(`repository-set-base-${seedData.repositoryId}`),
      addRepository,
      testPage.getByTestId("repository-set-reset-bases"),
    ]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }

    execSync("git update-ref refs/remotes/origin/main HEAD", {
      cwd: seedData.repositoryPath,
      env: makeGitEnv(backend.tmpDir),
    });
    const basePicker = testPage.getByTestId(`repository-set-base-${seedData.repositoryId}`);
    await basePicker.tap();
    const dropdown = testPage.getByTestId(`repository-set-base-dropdown-${seedData.repositoryId}`);
    await expect(dropdown).toBeVisible();
    await expect(dropdown.getByPlaceholder("Search branches...")).toBeVisible();
    await expect(dropdown.getByText("origin/main")).toBeVisible();
    await expect(dropdown.getByText("origin", { exact: true })).toBeVisible();

    const search = dropdown.getByPlaceholder("Search branches...");
    await search.fill("origin");
    await expect(dropdown.getByRole("option", { name: /^origin\/main origin/ })).toBeVisible();
    await expect(dropdown.getByRole("option", { name: /^main local/ })).toHaveCount(0);
    await dropdown.getByTestId("branch-refresh-button").tap();
    await expect(dropdown.getByRole("option", { name: /^origin\/main origin/ })).toBeVisible();

    const dropdownBox = await dropdown.boundingBox();
    const viewport = testPage.viewportSize();
    expect(dropdownBox).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(dropdownBox!.x).toBeGreaterThanOrEqual(0);
    expect(dropdownBox!.y).toBeGreaterThanOrEqual(0);
    expect(dropdownBox!.x + dropdownBox!.width).toBeLessThanOrEqual(viewport!.width);
    expect(dropdownBox!.y + dropdownBox!.height).toBeLessThanOrEqual(viewport!.height);
    await assertNoDocumentHorizontalOverflow(testPage, "mobile repository-set editor");
  });

  test("confirms deletion inline without a second overlay", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const setName = `Mobile settings set ${Date.now()}`;
    const created = await apiClient.createRepositorySet(seedData.workspaceId, setName, [
      seedData.repositoryId,
    ]);

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/repositories`);
    const row = testPage.getByTestId("repository-set-row");
    await expect(row).toContainText(setName);

    await row.getByTestId(`repository-set-delete-${created.id}`).tap();
    const inline = row.getByTestId("repository-set-delete-inline-confirmation");
    await expect(inline).toBeVisible();
    await expect(inline).toContainText(
      "The set is removed. Its repositories, and any task already using them, are not affected.",
    );
    await expect(testPage.getByTestId("repository-set-delete-confirm-popover")).toHaveCount(0);

    for (const control of [
      inline.getByTestId("repository-set-delete-confirm"),
      inline.getByRole("button", { name: "Cancel" }),
    ]) {
      const box = await control.boundingBox();
      expect(box).not.toBeNull();
      expect(box!.height).toBeGreaterThanOrEqual(44);
    }
    await assertNoDocumentHorizontalOverflow(testPage, "mobile repository-set confirmation");

    await inline.getByTestId("repository-set-delete-confirm").tap();
    await expect(testPage.getByTestId("repository-sets-empty")).toBeVisible();
    await expect
      .poll(async () => {
        const listed = await apiClient.listRepositorySets(seedData.workspaceId);
        return listed.repository_sets.some((entry) => entry.id === created.id);
      })
      .toBe(false);
  });
});
