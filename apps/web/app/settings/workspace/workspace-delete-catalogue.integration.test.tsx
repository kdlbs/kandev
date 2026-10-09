import { act, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import {
  ADDED,
  EDIT_PATH,
  REMOVED,
  SURVIVOR,
  TARGET,
  beginDelete,
  catalogue,
  choice,
  cleanupDelete,
  finishDelete,
  mountDelete,
  notify,
  openDelete,
  openPicker,
  row,
  select,
} from "./workspace-delete-catalogue.test-helpers";

afterEach(cleanupDelete);
const UPDATED_NAME = "Updated survivor";

// @covers AC-WORKSPACES-DELETION-002.1, AC-WORKSPACES-DELETION-002.2
it("preservesMixedLiveMembershipAfterAcceptedDelete", async () => {
  const fixture = await mountDelete();
  await beginDelete(fixture);
  notify(fixture, "workspace.created", row(ADDED, { name: "Live added choice" }));
  notify(fixture, "workspace.deleted", row(REMOVED));
  await openPicker();
  expect(choice(ADDED)?.textContent).toContain("Live added choice");
  expect(choice(ADDED)?.getAttribute("aria-disabled")).not.toBe("true");
  expect(choice(REMOVED)).toBeNull();
  expect(catalogue(fixture).map((item) => item.id)).toEqual([ADDED, TARGET, SURVIVOR]);
  await finishDelete(fixture);
  expect(catalogue(fixture).map((item) => item.id)).toEqual([ADDED, SURVIVOR]);
  await openPicker();
  expect(choice(ADDED)?.textContent).toContain("Live added choice");
  expect(choice(REMOVED)).toBeNull();
  expect(choice(TARGET)).toBeNull();
});

// @covers AC-WORKSPACES-DELETION-002.1, AC-WORKSPACES-DELETION-002.2
it("preservesSurvivorDescriptorsAfterAcceptedDelete", async () => {
  const fixture = await mountDelete();
  await beginDelete(fixture);
  notify(
    fixture,
    "workspace.updated",
    row(SURVIVOR, {
      name: UPDATED_NAME,
      description: "Current description",
      unit_id: "current-unit",
      default_executor_id: "current-executor",
      default_environment_id: "current-environment",
      default_agent_profile_id: "current-agent",
      default_config_agent_profile_id: "current-config",
      acp_idle_suspension_enabled: true,
      acp_idle_timeout_minutes: 47,
      updated_at: "2026-10-10T04:00:00Z",
    }),
  );
  const survivor = catalogue(fixture).find((item) => item.id === SURVIVOR)!;
  act(() =>
    fixture.store.getState().setWorkspaces(
      catalogue(fixture).map((item) =>
        item.id === SURVIVOR
          ? {
              ...item,
              owner_id: "current-owner",
              scopes: ["workspace.view"],
              created_at: "2026-10-03T00:00:00Z",
            }
          : item,
      ),
    ),
  );
  await openPicker();
  expect(choice(SURVIVOR)?.textContent).toContain(UPDATED_NAME);
  await finishDelete(fixture);
  expect(catalogue(fixture).find((item) => item.id === SURVIVOR)).toEqual({
    ...survivor,
    owner_id: "current-owner",
    scopes: ["workspace.view"],
    created_at: "2026-10-03T00:00:00Z",
  });
  expect(catalogue(fixture).find((item) => item.id === SURVIVOR)).toMatchObject({
    name: UPDATED_NAME,
    description: "Current description",
    unit_id: "current-unit",
    default_executor_id: "current-executor",
    default_environment_id: "current-environment",
    default_agent_profile_id: "current-agent",
    default_config_agent_profile_id: "current-config",
    acp_idle_suspension_enabled: true,
    acp_idle_timeout_minutes: 47,
    updated_at: "2026-10-10T04:00:00Z",
  });
  expect(choice(SURVIVOR)?.textContent).toContain(UPDATED_NAME);
});

// @covers AC-WORKSPACES-DELETION-002.3
it("preservesSelectionChangedDuringDelete", async () => {
  const fixture = await mountDelete();
  await beginDelete(fixture);
  const selected = select(fixture, SURVIVOR);
  await finishDelete(fixture);
  expect(fixture.store.getState().workspaces).toMatchObject({
    activeId: SURVIVOR,
    activeIdRevision: selected.activeIdRevision,
  });
});

// @covers AC-WORKSPACES-DELETION-002.3
it("retainsNullIdentityFallback", async () => {
  const fixture = await mountDelete();
  await beginDelete(fixture);
  const selected = select(fixture, null);
  await finishDelete(fixture);
  expect(fixture.store.getState().workspaces).toMatchObject({
    activeId: SURVIVOR,
    activeIdRevision: selected.activeIdRevision,
  });
});

// @covers AC-WORKSPACES-DELETION-002.1, AC-WORKSPACES-DELETION-002.3
it("preservesNotifiedTargetRemovalAndFallback", async () => {
  const fixture = await mountDelete();
  await beginDelete(fixture);
  notify(fixture, "workspace.deleted", row(TARGET));
  const selected = fixture.store.getState().workspaces;
  expect(selected).toMatchObject({ activeId: SURVIVOR, activeIdRevision: 12 });
  await finishDelete(fixture);
  expect(fixture.store.getState().workspaces).toEqual(selected);
});

// @covers AC-WORKSPACES-DELETION-002.2, AC-WORKSPACES-DELETION-002.4, AC-WORKSPACES-DELETION-002.5
it.each([false, true])("acceptsOrdinarySettingsDeletion (office=%s)", async (office) => {
  const fixture = await mountDelete({ office });
  const ticket = await beginDelete(fixture);
  expect(ticket.path).toBe(`/api/v1/${office ? "office/" : ""}workspaces/${TARGET}`);
  await finishDelete(fixture);
  expect(catalogue(fixture).map((item) => item.id)).toEqual([SURVIVOR, REMOVED]);
  expect(fixture.store.getState().workspaces).toMatchObject({
    activeId: TARGET,
    activeIdRevision: 11,
  });
  await openPicker();
  expect(choice(TARGET)).toBeNull();
  expect(choice(SURVIVOR)).not.toBeNull();
});

// @covers AC-WORKSPACES-DELETION-002.1, AC-WORKSPACES-DELETION-002.3, AC-WORKSPACES-DELETION-002.4
it("rejectedDeletePreservesLiveCatalogueAndConfirmation", async () => {
  const fixture = await mountDelete();
  await beginDelete(fixture);
  notify(fixture, "workspace.created", row(ADDED));
  notify(fixture, "workspace.deleted", row(REMOVED));
  const selected = select(fixture, SURVIVOR);
  await finishDelete(fixture, 503);
  expect(catalogue(fixture).map((item) => item.id)).toEqual([ADDED, TARGET, SURVIVOR]);
  expect(fixture.store.getState().workspaces).toMatchObject({
    activeId: SURVIVOR,
    activeIdRevision: selected.activeIdRevision,
  });
  expect(window.location.pathname).toBe(EDIT_PATH);
  expect(screen.getByTestId("workspace-settings-delete-confirm-input")).toHaveProperty(
    "value",
    "Target workspace",
  );
  await openPicker();
  expect(choice(ADDED)).not.toBeNull();
  expect(choice(REMOVED)).toBeNull();
});

// @covers AC-WORKSPACES-DELETION-002.5
it("mismatchedConfirmationDoesNotDelete", async () => {
  const fixture = await mountDelete();
  await openDelete("target workspace");
  const confirm = screen.getByTestId("workspace-settings-delete-confirm-button");
  expect(confirm).toHaveProperty("disabled", true);
  fireEvent.click(confirm);
  expect(fixture.tickets).toEqual([]);
  fireEvent.change(screen.getByTestId("workspace-settings-delete-confirm-input"), {
    target: { value: "Target workspace" },
  });
  expect(confirm).toHaveProperty("disabled", false);
  fireEvent.click(confirm);
  await finishDelete(fixture);
  expect(catalogue(fixture).map((item) => item.id)).toEqual([SURVIVOR, REMOVED]);
});

// @covers AC-WORKSPACES-DELETION-002.5
it("hidesDeleteWithoutManageScope", async () => {
  const fixture = await mountDelete({
    items: [row(TARGET, { scopes: ["workspace.view"] }), row(SURVIVOR)],
  });
  expect(screen.queryByTestId("workspace-settings-delete-button")).toBeNull();
  expect(fixture.tickets).toEqual([]);
});

// @covers AC-WORKSPACES-DELETION-002.3, AC-WORKSPACES-DELETION-002.4
it("acceptsLastWorkspaceDeletionWithoutNewSelectionPolicy", async () => {
  const fixture = await mountDelete({ items: [row(TARGET)] });
  await beginDelete(fixture);
  await finishDelete(fixture);
  expect(fixture.store.getState().workspaces).toEqual({
    items: [],
    activeId: TARGET,
    activeIdRevision: 11,
  });
});
