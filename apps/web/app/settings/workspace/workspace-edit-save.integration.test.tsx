import { act, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import {
  ACCEPTED_NAME,
  CREATED_ID,
  NEWER_NAME,
  OTHER_ID,
  SETTINGS_PATH,
  TARGET_ID,
  assertGuardedRoute,
  beginSave,
  catalogue,
  choice,
  chooseDefault,
  cleanupWorkspaceFixture,
  editName,
  finishSave,
  mountWorkspace,
  notify,
  openPicker,
  workspace,
} from "./workspace-edit-save.test-helpers";
import type { WorkspaceUpdates } from "./workspace-edit-save";

afterEach(cleanupWorkspaceFixture);

const LIVE_OTHER_NAME = "Live renamed other";

// @covers AC-WORKSPACES-SETTINGS-UPDATES-002.1 AC-WORKSPACES-SETTINGS-UPDATES-002.2
it("preservesCreatedWorkspaceChoiceWhenSaveIsAcknowledged", async () => {
  const current = await mountWorkspace();
  editName();
  const ticket = await beginSave(current);
  notify(current, "workspace.created", {
    id: CREATED_ID,
    name: "Live new workspace",
    owner_id: "live-owner",
  });
  await openPicker();
  expect(catalogue(current).map((row) => row.id)).toEqual([CREATED_ID, TARGET_ID, OTHER_ID]);
  expect(choice(CREATED_ID)?.textContent).toContain("Live new workspace");
  const result = await finishSave(current, ticket);
  expect(result.canLeave).toBe(true);
  expect(catalogue(current).find((row) => row.id === TARGET_ID)?.name).toBe(ACCEPTED_NAME);
  // Both independently observable consequences must be reached in RED.
  expect.soft(catalogue(current).map((row) => row.id)).toEqual([CREATED_ID, TARGET_ID, OTHER_ID]);
  expect.soft(choice(CREATED_ID)?.textContent ?? "").toContain("Live new workspace");
  expect(choice(TARGET_ID)?.textContent).toContain(ACCEPTED_NAME);
  expect(choice(CREATED_ID)).not.toBeNull();
  fireEvent.click(choice(CREATED_ID)!);
  expect(current.store.getState().workspaces.activeId).toBe(CREATED_ID);
  expect(window.location.pathname).toBe("/");
  expect(new URLSearchParams(window.location.search).get("home")).toBe("overview");
  expect(new URLSearchParams(window.location.search).get("workspaceId")).toBe(CREATED_ID);
});

// @covers AC-WORKSPACES-SETTINGS-UPDATES-002.1 AC-WORKSPACES-SETTINGS-UPDATES-002.2
it("preservesUpdatedOtherWorkspaceChoiceWhenSaveIsAcknowledged", async () => {
  const current = await mountWorkspace();
  editName();
  const ticket = await beginSave(current);
  notify(current, "workspace.updated", {
    id: OTHER_ID,
    name: LIVE_OTHER_NAME,
    description: "Live description",
    default_executor_id: "live-executor",
    unit_id: "live-unit",
    updated_at: "2026-10-09T00:00:00Z",
  });
  await openPicker();
  expect(catalogue(current).find((row) => row.id === OTHER_ID)).toMatchObject({
    name: LIVE_OTHER_NAME,
    description: "Live description",
  });
  expect(choice(OTHER_ID)?.textContent).toContain(LIVE_OTHER_NAME);
  await finishSave(current, ticket);
  expect(catalogue(current).find((row) => row.id === TARGET_ID)?.name).toBe(ACCEPTED_NAME);
  expect.soft(catalogue(current).find((row) => row.id === OTHER_ID)).toMatchObject({
    name: LIVE_OTHER_NAME,
    description: "Live description",
    default_executor_id: "live-executor",
    unit_id: "live-unit",
    updated_at: "2026-10-09T00:00:00Z",
  });
  expect.soft(choice(OTHER_ID)?.textContent).toContain(LIVE_OTHER_NAME);
  expect(catalogue(current).map((row) => row.id)).toEqual([TARGET_ID, OTHER_ID]);
});

// @covers AC-WORKSPACES-SETTINGS-UPDATES-002.1 AC-WORKSPACES-SETTINGS-UPDATES-002.2
it("preservesRemovedOtherWorkspaceChoiceWhenSaveIsAcknowledged", async () => {
  const current = await mountWorkspace();
  editName();
  const ticket = await beginSave(current);
  notify(current, "workspace.deleted", { id: OTHER_ID, name: "Original other" });
  await openPicker();
  expect(catalogue(current).map((row) => row.id)).toEqual([TARGET_ID]);
  expect(choice(OTHER_ID)).toBeNull();
  await finishSave(current, ticket);
  expect(catalogue(current).find((row) => row.id === TARGET_ID)?.name).toBe(ACCEPTED_NAME);
  expect.soft(catalogue(current).map((row) => row.id)).toEqual([TARGET_ID]);
  expect.soft(choice(OTHER_ID)).toBeNull();
  expect(choice(TARGET_ID)?.textContent).toContain(ACCEPTED_NAME);
});

// @covers AC-WORKSPACES-SETTINGS-UPDATES-002.3
it("preservesCurrentTargetMetadataAndActiveSelection", async () => {
  const current = await mountWorkspace();
  editName();
  const ticket = await beginSave(current);
  act(() => {
    const state = current.store.getState();
    state.hydrate({
      workspaces: {
        ...state.workspaces,
        items: state.workspaces.items.map((row) =>
          row.id === TARGET_ID
            ? {
                ...row,
                owner_id: "live-owner",
                scopes: ["workspace.manage", "member.manage"],
                created_at: "2026-10-03T00:00:00Z",
              }
            : row,
        ),
      },
    });
    current.store.getState().setActiveWorkspace(OTHER_ID);
  });
  notify(current, "workspace.updated", {
    id: TARGET_ID,
    name: "Live target event",
    description: "Live target description",
    unit_id: "live-target-unit",
    default_config_agent_profile_id: "live-config",
    updated_at: "2026-10-09T01:00:00Z",
  });
  expect(current.store.getState().workspaces).toMatchObject({
    activeId: OTHER_ID,
    activeIdRevision: 8,
  });
  expect(catalogue(current).find((row) => row.id === TARGET_ID)?.description).toBe(
    "Live target description",
  );
  await finishSave(
    current,
    ticket,
    workspace(TARGET_ID, {
      name: ACCEPTED_NAME,
      default_executor_id: "accepted-executor",
      default_environment_id: "accepted-environment",
      default_agent_profile_id: "accepted-profile",
      acp_idle_suspension_enabled: true,
      acp_idle_timeout_minutes: 55,
    }),
  );
  expect(catalogue(current).find((row) => row.id === TARGET_ID)).toMatchObject({
    name: ACCEPTED_NAME,
    default_executor_id: "accepted-executor",
    default_environment_id: "accepted-environment",
    default_agent_profile_id: "accepted-profile",
    acp_idle_suspension_enabled: true,
    acp_idle_timeout_minutes: 55,
    description: "Live target description",
    owner_id: "live-owner",
    scopes: ["workspace.manage", "member.manage"],
    unit_id: "live-target-unit",
    default_config_agent_profile_id: "live-config",
    created_at: "2026-10-03T00:00:00Z",
    updated_at: "2026-10-09T01:00:00Z",
  });
  expect(current.store.getState().workspaces).toMatchObject({
    activeId: OTHER_ID,
    activeIdRevision: 8,
  });
});

// @covers AC-WORKSPACES-SETTINGS-UPDATES-002.3 AC-WORKSPACES-SETTINGS-UPDATES-002.4
it.each(["complete", "omitted"])(
  "acceptsOrdinarySaveAndClearsContributor: %s",
  async (responseShape) => {
    const current = await mountWorkspace();
    editName();
    const ticket = await beginSave(current);
    expect(ticket.updates).toEqual({ name: ACCEPTED_NAME });
    const response = workspace(TARGET_ID, { name: ACCEPTED_NAME });
    if (responseShape === "omitted") {
      delete response.default_executor_id;
      delete response.default_environment_id;
      delete response.default_agent_profile_id;
      delete response.acp_idle_suspension_enabled;
      delete response.acp_idle_timeout_minutes;
    }
    const result = await finishSave(current, ticket, response);
    expect(result).toMatchObject({ canLeave: true, failedIds: new Set() });
    expect(current.coordinator.hasDirty).toBe(false);
    expect(current.coordinator.contributorStates).toContainEqual({
      id: `workspace:${TARGET_ID}`,
      isDirty: false,
      invalid: false,
      saveFailed: false,
    });
    expect(catalogue(current).find((row) => row.id === TARGET_ID)).toMatchObject({
      name: ACCEPTED_NAME,
      default_executor_id: null,
      default_environment_id: null,
      default_agent_profile_id: null,
      acp_idle_suspension_enabled: response.acp_idle_suspension_enabled,
      acp_idle_timeout_minutes: response.acp_idle_timeout_minutes,
    });
    expect(window.location.pathname).toBe(SETTINGS_PATH);
    editName(`  ${ACCEPTED_NAME}  `);
    expect(current.coordinator.hasDirty).toBe(false);
    await act(async () => {
      await current.coordinator.saveAll();
    });
    expect(current.tickets).toHaveLength(1);
    fireEvent.click(screen.getByTestId("leave-settings"));
    expect(window.location.pathname).toBe("/settings/workspaces");
  },
);

// @covers AC-WORKSPACES-SETTINGS-UPDATES-002.4 AC-WORKSPACES-SETTINGS-UPDATES-002.6
it("doesNotPatchPristineWorkspace", async () => {
  const current = await mountWorkspace();
  await act(async () => {
    expect((await current.coordinator.saveAll()).canLeave).toBe(true);
  });
  expect(current.tickets).toEqual([]);
  expect(current.coordinator.hasDirty).toBe(false);
});

// @covers AC-WORKSPACES-SETTINGS-UPDATES-002.4
it("acceptedSaveRetainsNewerDraftAndRouteGuard", async () => {
  const current = await mountWorkspace();
  editName();
  const ticket = await beginSave(current);
  editName(NEWER_NAME);
  const result = await finishSave(current, ticket);
  expect(result.canLeave).toBe(false);
  expect(catalogue(current).find((row) => row.id === TARGET_ID)?.name).toBe(ACCEPTED_NAME);
  expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe(NEWER_NAME);
  expect(current.coordinator.hasDirty).toBe(true);
  await assertGuardedRoute();
});

// @covers AC-WORKSPACES-SETTINGS-UPDATES-002.5
it("rejectedSavePreservesCurrentCatalogueNewerDraftAndRoute", async () => {
  const current = await mountWorkspace();
  editName();
  const ticket = await beginSave(current);
  notify(current, "workspace.created", { id: CREATED_ID, name: "Retained live workspace" });
  editName(NEWER_NAME);
  const result = await finishSave(current, ticket, workspace(TARGET_ID), 500);
  expect(result).toMatchObject({
    canLeave: false,
    failedIds: new Set([`workspace:${TARGET_ID}`]),
  });
  expect(catalogue(current).find((row) => row.id === TARGET_ID)?.name).toBe("Original target");
  expect(catalogue(current).map((row) => row.id)).toEqual([CREATED_ID, TARGET_ID, OTHER_ID]);
  expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe(NEWER_NAME);
  expect(current.coordinator.contributorStates).toContainEqual({
    id: `workspace:${TARGET_ID}`,
    isDirty: true,
    invalid: false,
    saveFailed: true,
  });
  expect(screen.getByTestId("toast-message").textContent).toContain("Settings save rejected");
  await openPicker();
  expect(choice(CREATED_ID)?.textContent).toContain("Retained live workspace");
  fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
  await assertGuardedRoute();
  fireEvent.click(screen.getByRole("button", { name: "Continue editing" }));
  editName("Original target");
  expect(current.coordinator.hasDirty).toBe(false);
});

// @covers AC-WORKSPACES-SETTINGS-UPDATES-002.6
const payloadCases: {
  name: string;
  initial?: Parameters<typeof mountWorkspace>[0];
  change: () => Promise<void> | void;
  expected: WorkspaceUpdates;
}[] = [
  { name: "trimmed name", change: () => editName("  Renamed  "), expected: { name: "Renamed" } },
  {
    name: "executor selection",
    change: () => chooseDefault(0, "Local executor"),
    expected: { default_executor_id: "local-executor" },
  },
  {
    name: "agent selection",
    change: () => chooseDefault(1, "Agent profile"),
    expected: { default_agent_profile_id: "agent-profile" },
  },
  {
    name: "executor clear",
    initial: { default_executor_id: "local-executor" },
    change: () => chooseDefault(0, "No default"),
    expected: { default_executor_id: "" },
  },
  {
    name: "agent clear",
    initial: { default_agent_profile_id: "agent-profile" },
    change: () => chooseDefault(1, "No default"),
    expected: { default_agent_profile_id: "" },
  },
  {
    name: "explicit false",
    initial: { acp_idle_suspension_enabled: true },
    change: () => {
      fireEvent.click(screen.getByTestId("workspace-idle-suspension-switch"));
    },
    expected: { acp_idle_suspension_enabled: false },
  },
  {
    name: "numeric timeout",
    initial: { acp_idle_suspension_enabled: true },
    change: () => {
      fireEvent.change(screen.getByTestId("workspace-idle-timeout-input"), {
        target: { value: "45" },
      });
    },
    expected: { acp_idle_timeout_minutes: 45 },
  },
  {
    name: "mixed changed fields",
    initial: { acp_idle_suspension_enabled: true },
    change: () => {
      editName("  Mixed name  ");
      fireEvent.change(screen.getByTestId("workspace-idle-timeout-input"), {
        target: { value: "60" },
      });
      fireEvent.click(screen.getByTestId("workspace-idle-suspension-switch"));
    },
    expected: {
      name: "Mixed name",
      acp_idle_timeout_minutes: 60,
      acp_idle_suspension_enabled: false,
    },
  },
];

it.each(payloadCases)(
  "sendsOnlyChangedWorkspaceSettings: $name",
  async ({ initial, change, expected }) => {
    const current = await mountWorkspace(initial);
    await change();
    const ticket = await beginSave(current);
    expect(ticket.updates).toEqual(expected);
    expect(Object.keys(ticket.updates).sort()).toEqual(Object.keys(expected).sort());
    await finishSave(current, ticket, workspace(TARGET_ID, { ...initial, ...expected }));
  },
);

// @covers AC-WORKSPACES-SETTINGS-UPDATES-002.6
it.each(["blank name", "invalid timeout", "unmanaged"])(
  "invalidOrUnmanagedWorkspaceDoesNotPatch: %s",
  async (condition) => {
    const current = await mountWorkspace(
      condition === "unmanaged" ? { scopes: [] } : { acp_idle_suspension_enabled: true },
    );
    if (condition === "unmanaged") {
      expect((screen.getByLabelText("Name") as HTMLInputElement).disabled).toBe(true);
      expect(
        (screen.getByTestId("workspace-idle-suspension-switch") as HTMLButtonElement).disabled,
      ).toBe(true);
    } else if (condition === "blank name") editName(" ");
    else
      fireEvent.change(screen.getByTestId("workspace-idle-timeout-input"), {
        target: { value: "0" },
      });
    await act(async () => {
      await current.coordinator.saveAll();
    });
    expect(current.tickets).toEqual([]);
  },
);
