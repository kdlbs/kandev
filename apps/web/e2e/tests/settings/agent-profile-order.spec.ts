import { test, expect } from "../../fixtures/test-base";
import {
  seedProfileOrder,
  profileRow,
  pointerDragProfile,
  visibleProfileNames,
  restoreProfileOrder,
} from "./agent-profile-order-helpers";

/** Fragment of the translated profile-position announcement. */
const MOVED_OVER = "moved to position";

test.describe("Agent profile ordering", () => {
  test("pointer reorder persists across clients and Settings navigation while membership changes stay first", async ({
    testPage,
    apiClient,
    backend,
  }) => {
    test.setTimeout(120_000);
    const { original, current, createdIds } = await seedProfileOrder(apiClient, "Pointer");
    const other = await testPage.context().newPage();
    try {
      await testPage.addInitScript(() =>
        window.localStorage.setItem("kandev.settings.menuMode", JSON.stringify("accordion")),
      );
      await testPage.goto("/settings/agents");
      await other.goto("/settings/agents");
      const group = testPage.getByTestId(`agent-profiles-${original.name}`);
      const otherGroup = other.getByTestId(`agent-profiles-${original.name}`);
      await expect(group.getByTestId("agent-profile-row")).toHaveCount(current.profiles.length);
      await expect(otherGroup.getByTestId("agent-profile-row")).toHaveCount(
        current.profiles.length,
      );
      const [source, target, ...rest] = current.profiles;
      const expected = [target, source, ...rest];
      await pointerDragProfile(
        testPage,
        profileRow(testPage, original, source.id),
        profileRow(testPage, original, target.id),
      );
      await expect
        .poll(async () =>
          (await apiClient.listAgents()).agents
            .find((agent) => agent.id === original.id)
            ?.profiles.map((profile) => profile.id),
        )
        .toEqual(expected.map((profile) => profile.id));
      await expect
        .poll(() => visibleProfileNames(group))
        .toEqual(expected.map((profile) => profile.name));
      await expect
        .poll(() => visibleProfileNames(otherGroup))
        .toEqual(expected.map((profile) => profile.name));
      const tree = testPage.getByTestId("app-sidebar-settings-mode");
      await tree.getByRole("button", { name: "Expand Mock", exact: true }).click();
      await expect
        .poll(() =>
          tree.locator(`a[href^="/settings/agents/${original.name}/profiles/"]`).allTextContents(),
        )
        .toEqual(expected.map((profile) => profile.name));
      await testPage.reload();
      await expect
        .poll(() => visibleProfileNames(group))
        .toEqual(expected.map((profile) => profile.name));

      const added = await apiClient.createAgentProfile(original.id, `New ${Date.now()}`, {
        model: "mock-fast",
      });
      createdIds.push(added.id);
      await expect
        .poll(() => visibleProfileNames(otherGroup))
        .toEqual([added.name, ...expected.map((profile) => profile.name)]);
      await profileRow(testPage, original, added.id)
        .getByTestId(`duplicate-profile-inline-${added.id}`)
        .click();
      let copyId = "";
      await expect
        .poll(async () => {
          const profiles = (await apiClient.listAgents()).agents.find(
            (agent) => agent.id === original.id,
          )!.profiles;
          copyId = profiles.find((profile) => profile.name === `${added.name} Copy`)?.id ?? "";
          return profiles[0]?.name;
        })
        .toBe(`${added.name} Copy`);
      createdIds.push(copyId);
      await expect
        .poll(() => visibleProfileNames(otherGroup))
        .toEqual([`${added.name} Copy`, added.name, ...expected.map((profile) => profile.name)]);
      const copyRow = profileRow(testPage, original, copyId);
      await copyRow.getByTestId(`delete-profile-inline-${copyId}`).click();
      await testPage.getByTestId("agent-profile-delete-confirm").click();
      await expect(copyRow).toHaveCount(0);
      await expect
        .poll(() => visibleProfileNames(otherGroup))
        .toEqual([added.name, ...expected.map((profile) => profile.name)]);
      await profileRow(testPage, original, added.id).getByTestId("agent-profile-row-link").click();
      await expect(testPage).toHaveURL(new RegExp(`/profiles/${added.id}$`));
    } finally {
      await other.close();
      for (const id of createdIds)
        await apiClient.deleteAgentProfile(id, true).catch(() => undefined);
      await restoreProfileOrder(testPage, backend.baseUrl, original);
    }
  });

  test("keyboard reorder saves and reloads while Escape cancels", async ({
    testPage,
    apiClient,
    backend,
  }, testInfo) => {
    const { original, current, createdIds } = await seedProfileOrder(apiClient, "Keyboard");
    try {
      await testPage.goto("/settings/agents");
      const [source, target, ...rest] = current.profiles;
      const row = profileRow(testPage, original, source.id);
      const handle = row.getByTestId("agent-profile-drag-handle");
      await expect(handle).toBeVisible();
      await expect
        .poll(async () => {
          const box = await handle.boundingBox();
          return box && [box.height, box.width];
        })
        .toEqual([expect.closeTo(28, 0), expect.closeTo(28, 0)]);
      await expect(handle).toHaveCSS("cursor", "pointer");
      await testInfo.attach("profile-order-desktop", {
        body: await testPage.screenshot({ fullPage: true }),
        contentType: "image/png",
      });
      await handle.focus();
      await handle.press("Space");
      await expect(row.locator("xpath=..")).toHaveClass(/opacity-70/);
      const announcedTarget = async () =>
        (await testPage.locator('[id^="DndLiveRegion"]').allTextContents()).find((text) =>
          text.includes("Profile"),
        ) ?? "";
      await expect.poll(announcedTarget).toContain(MOVED_OVER);
      const initialAnnouncement = await announcedTarget();
      await handle.press("ArrowDown");
      await expect.poll(announcedTarget).toContain(MOVED_OVER);
      await expect.poll(announcedTarget).not.toBe(initialAnnouncement);
      await handle.press("Escape");
      await expect(row.locator("xpath=..")).not.toHaveClass(/opacity-70/);
      await expect
        .poll(() => visibleProfileNames(testPage.getByTestId(`agent-profiles-${original.name}`)))
        .toEqual(current.profiles.map((profile) => profile.name));
      await handle.press("Space");
      await expect(row.locator("xpath=..")).toHaveClass(/opacity-70/);
      await handle.press("ArrowDown");
      await expect.poll(announcedTarget).not.toBe(initialAnnouncement);
      await handle.press("Space");
      await expect
        .poll(async () =>
          (await apiClient.listAgents()).agents
            .find((agent) => agent.id === original.id)
            ?.profiles.map((profile) => profile.id),
        )
        .toEqual([target, source, ...rest].map((profile) => profile.id));
      await testPage.reload();
      await expect
        .poll(() => visibleProfileNames(testPage.getByTestId(`agent-profiles-${original.name}`)))
        .toEqual([target, source, ...rest].map((profile) => profile.name));
    } finally {
      for (const id of createdIds)
        await apiClient.deleteAgentProfile(id, true).catch(() => undefined);
      await restoreProfileOrder(testPage, backend.baseUrl, original);
    }
  });
});

test("dropping on another agent leaves both profile orders unchanged", async ({
  testPage,
  apiClient,
  backend,
}) => {
  const { original, current, createdIds } = await seedProfileOrder(apiClient, "Cross-agent", 3);
  const otherBase = await apiClient.createCustomTUIAgent({
    display_name: `Cross-agent ${Date.now()}`,
    command: "cat",
  });
  const otherProfile = await apiClient.createAgentProfile(otherBase.id, "Other agent profile", {
    model: "mock-fast",
  });
  const other = (await apiClient.listAgents()).agents.find((agent) => agent.id === otherBase.id)!;
  try {
    await testPage.setViewportSize({ width: 1440, height: 1400 });
    await testPage.goto("/settings/agents");
    const [source, target, ...rest] = current.profiles;
    await pointerDragProfile(
      testPage,
      profileRow(testPage, original, source.id),
      profileRow(testPage, original, target.id),
    );
    const expected = [target, source, ...rest];
    await expect
      .poll(async () =>
        (await apiClient.listAgents()).agents
          .find((agent) => agent.id === original.id)
          ?.profiles.map((profile) => profile.id),
      )
      .toEqual(expected.map((profile) => profile.id));
    await pointerDragProfile(
      testPage,
      profileRow(testPage, original, target.id),
      profileRow(testPage, other, otherProfile.id),
    );
    await expect
      .poll(() => visibleProfileNames(testPage.getByTestId(`agent-profiles-${original.name}`)))
      .toEqual(expected.map((profile) => profile.name));
    const after = (await apiClient.listAgents()).agents;
    expect(
      after.find((agent) => agent.id === original.id)?.profiles.map((profile) => profile.id),
    ).toEqual(expected.map((profile) => profile.id));
    expect(
      after.find((agent) => agent.id === other.id)?.profiles.map((profile) => profile.id),
    ).toEqual(other.profiles.map((profile) => profile.id));
  } finally {
    await apiClient.deleteCustomAgentByName(other.name);
    for (const id of createdIds) await apiClient.deleteAgentProfile(id, true);
    await restoreProfileOrder(testPage, backend.baseUrl, original);
  }
});
