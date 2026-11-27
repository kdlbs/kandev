import { act, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
  ACCEPTED,
  BASE,
  INDEPENDENT,
  OTHER,
  REJECTION,
  beginCreation,
  catalogue,
  choice,
  cleanupCatalogue,
  descriptor,
  displayedNames,
  finishCreation,
  managementChoice,
  mountCatalogue,
  notify,
  openForm,
  openPicker,
} from "./workspaces-create.test-helpers";

afterEach(cleanupCatalogue);

const acceptedName = "Accepted workspace";
const NAME_LABEL = "Workspace Name";
const LABEL_ATTRIBUTE = "aria-label";
const ADDED_NAME = "Independent added";
const RENAMED_NAME = "Independent renamed";
const CREATED_EVENT = "workspace.created";
const UPDATED_EVENT = "workspace.updated";
const MANAGEMENT_LINK = "workspace-overview-link";

function expectChoice(id: string, name: string) {
  expect(choice(id)?.textContent).toContain(name);
  expect(managementChoice(id)?.getAttribute(LABEL_ATTRIBUTE)).toBe(name);
}

// @covers AC-WORKSPACES-SETTINGS-UPDATES-003.1 AC-WORKSPACES-SETTINGS-UPDATES-003.3
describe("independent catalogue changes during acknowledged creation", () => {
  it("preservesIndependentCreatedChoiceAtAcknowledgement", async () => {
    const current = await mountCatalogue();
    await beginCreation(current);
    notify(current, CREATED_EVENT, {
      id: INDEPENDENT,
      name: ADDED_NAME,
      owner_id: "independent-owner",
    });
    await openPicker();
    expect(catalogue(current).items.map((row) => row.id)).toEqual([INDEPENDENT, BASE, OTHER]);
    expectChoice(INDEPENDENT, ADDED_NAME);
    await finishCreation(current, descriptor(ACCEPTED, { name: acceptedName }));
    expect(catalogue(current).items.find((row) => row.id === ACCEPTED)?.name).toBe(acceptedName);
    expect
      .soft(catalogue(current).items.map((row) => row.id))
      .toEqual([ACCEPTED, INDEPENDENT, BASE, OTHER]);
    expect.soft(choice(INDEPENDENT)?.textContent ?? "").toContain(ADDED_NAME);
    expect.soft(managementChoice(INDEPENDENT)?.getAttribute(LABEL_ATTRIBUTE)).toBe(ADDED_NAME);
    expectChoice(ACCEPTED, acceptedName);
    expect(choice(INDEPENDENT)).not.toBeNull();
    fireEvent.click(choice(INDEPENDENT)!);
    expect(catalogue(current).activeId).toBe(INDEPENDENT);
    expect(catalogue(current).activeIdRevision).toBe(8);
    expect(window.location.pathname).toBe("/");
    expect(new URLSearchParams(window.location.search).get("workspaceId")).toBe(INDEPENDENT);
  });

  it("preservesIndependentUpdatedChoiceAtAcknowledgement", async () => {
    const current = await mountCatalogue();
    await beginCreation(current);
    notify(current, UPDATED_EVENT, {
      id: OTHER,
      name: RENAMED_NAME,
      description: "Changed description",
      unit_id: "changed-unit",
      default_executor_id: "changed-executor",
      updated_at: "2026-10-09T07:00:00Z",
    });
    const changed = catalogue(current).items.find((row) => row.id === OTHER);
    await openPicker();
    expectChoice(OTHER, RENAMED_NAME);
    await finishCreation(current);
    expect(catalogue(current).items[0].id).toBe(ACCEPTED);
    expect.soft(catalogue(current).items.find((row) => row.id === OTHER)).toEqual(changed);
    expect.soft(choice(OTHER)?.textContent ?? "").toContain(RENAMED_NAME);
    expect.soft(managementChoice(OTHER)?.getAttribute(LABEL_ATTRIBUTE)).toBe(RENAMED_NAME);
    expect(catalogue(current).items.map((row) => row.id)).toEqual([ACCEPTED, BASE, OTHER]);
  });

  it("preservesIndependentDeletionAtAcknowledgement", async () => {
    const current = await mountCatalogue();
    await beginCreation(current);
    notify(current, "workspace.deleted", { id: OTHER, name: OTHER });
    await openPicker();
    expect(catalogue(current).items.map((row) => row.id)).toEqual([BASE]);
    expect(choice(OTHER)).toBeNull();
    expect(managementChoice(OTHER)).toBeNull();
    await finishCreation(current);
    expect(catalogue(current).items[0].id).toBe(ACCEPTED);
    expect.soft(catalogue(current).items.map((row) => row.id)).toEqual([ACCEPTED, BASE]);
    expect.soft(choice(OTHER)).toBeNull();
    expect.soft(managementChoice(OTHER)).toBeNull();
  });

  it("appliesIndependentNotificationsAfterAcknowledgement", async () => {
    const current = await mountCatalogue();
    await beginCreation(current);
    await finishCreation(current);
    notify(current, CREATED_EVENT, { id: INDEPENDENT, name: "Later addition" });
    notify(current, UPDATED_EVENT, {
      id: OTHER,
      name: "Later rename",
      description: "Later description",
    });
    notify(current, "workspace.deleted", { id: BASE, name: BASE });
    await openPicker();
    expect(catalogue(current).items.map((row) => row.id)).toEqual([INDEPENDENT, ACCEPTED, OTHER]);
    expect(catalogue(current)).toMatchObject({ activeId: INDEPENDENT, activeIdRevision: 8 });
    expectChoice(INDEPENDENT, "Later addition");
    expectChoice(OTHER, "Later rename");
    expect(catalogue(current).items.find((row) => row.id === OTHER)?.description).toBe(
      "Later description",
    );
    expect(choice(BASE)).toBeNull();
    expect(managementChoice(BASE)).toBeNull();
  });
});

// @covers AC-WORKSPACES-SETTINGS-UPDATES-003.2 AC-WORKSPACES-SETTINGS-UPDATES-003.3
describe("accepted identity and descriptors", () => {
  it.each(["before", "after"])("publishesAcceptedIdentityOnce: WS %s ACK", async (timing) => {
    const current = await mountCatalogue();
    await beginCreation(current);
    const event = { id: ACCEPTED, name: acceptedName, owner_id: "response-owner" };
    if (timing === "before") notify(current, CREATED_EVENT, event);
    await finishCreation(
      current,
      descriptor(ACCEPTED, { name: acceptedName, owner_id: "response-owner" }),
    );
    if (timing === "after") notify(current, CREATED_EVENT, event);
    await openPicker();
    expect(catalogue(current).items.map((row) => row.id)).toEqual([ACCEPTED, BASE, OTHER]);
    expect(screen.getAllByTestId(`sidebar-workspace-item-${ACCEPTED}`)).toHaveLength(1);
    expect(
      screen
        .getAllByTestId(MANAGEMENT_LINK)
        .filter((row) => row.getAttribute(LABEL_ATTRIBUTE) === acceptedName),
    ).toHaveLength(1);
    expect(catalogue(current).items[0]).toMatchObject({
      name: acceptedName,
      owner_id: "response-owner",
      scopes: ["workspace.manage"],
      viewer_role: "owner",
      member_count: 3,
    });
    if (timing === "before")
      expect(catalogue(current).items[0].default_executor_id).toBe("executor-id");
    else expect(catalogue(current).items[0].default_executor_id).toBeNull();
  });

  // @covers AC-WORKSPACES-SETTINGS-UPDATES-003.4
  it.each(["complete", "omitted"])("acceptsOrdinaryCreation: %s descriptor", async (shape) => {
    const existing = [
      descriptor(BASE, { description: "Current other metadata" }),
      descriptor(OTHER),
    ];
    const current = await mountCatalogue(existing);
    const cookie = document.cookie;
    const ticket = await beginCreation(current);
    expect(ticket.payload).toEqual({ name: acceptedName });
    const response = descriptor(ACCEPTED, {
      name: acceptedName,
      office_workflow_id: "office-workflow",
    });
    if (shape === "omitted") {
      for (const key of [
        "description",
        "default_executor_id",
        "default_environment_id",
        "default_agent_profile_id",
        "default_config_agent_profile_id",
        "acp_idle_suspension_enabled",
        "acp_idle_timeout_minutes",
        "office_workflow_id",
      ] as const)
        delete response[key];
    }
    await finishCreation(current, response);
    const accepted = catalogue(current).items[0];
    expect(accepted).toEqual({
      id: ACCEPTED,
      name: acceptedName,
      description: shape === "complete" ? ACCEPTED : null,
      owner_id: "owner-id",
      unit_id: "unit-id",
      viewer_role: "owner",
      scopes: ["workspace.manage"],
      member_count: 3,
      default_executor_id: shape === "complete" ? "executor-id" : null,
      default_environment_id: shape === "complete" ? "environment-id" : null,
      default_agent_profile_id: shape === "complete" ? "agent-profile-id" : null,
      default_config_agent_profile_id: shape === "complete" ? "config-profile-id" : null,
      acp_idle_suspension_enabled: shape === "complete",
      acp_idle_timeout_minutes: shape === "complete" ? 45 : 120,
      office_workflow_id: shape === "complete" ? "office-workflow" : null,
      created_at: "2026-10-01T00:00:00Z",
      updated_at: "2026-10-02T00:00:00Z",
    });
    expect(catalogue(current).items.slice(1)).toEqual(existing);
    expect(catalogue(current)).toMatchObject({ activeId: BASE, activeIdRevision: 7 });
    expect(displayedNames()).toEqual([BASE, acceptedName, OTHER]);
    await openPicker();
    expectChoice(ACCEPTED, acceptedName);
    expect(document.cookie).toBe(cookie);
    expect(window.location.pathname).toBe("/settings/workspaces");
    fireEvent.keyDown(screen.getByRole("menu"), { key: "Escape" });
    openForm();
    expect((screen.getByLabelText(NAME_LABEL) as HTMLInputElement).value).toBe("");
    expect(current.countRequests).toBeGreaterThan(0);
  });
});

// @covers AC-WORKSPACES-SETTINGS-UPDATES-003.3 AC-WORKSPACES-SETTINGS-UPDATES-003.4
describe("selection and ownership controls", () => {
  it("createsInEmptyCatalogue", async () => {
    const current = await mountCatalogue([], null, 0);
    await beginCreation(current);
    await finishCreation(current);
    expect(catalogue(current)).toMatchObject({ activeId: ACCEPTED, activeIdRevision: 0 });
    expect(catalogue(current).items.map((row) => row.id)).toEqual([ACCEPTED]);
    await openPicker();
    expectChoice(ACCEPTED, ACCEPTED);
  });

  it("preservesSelectionChangedDuringCreation", async () => {
    const current = await mountCatalogue();
    await beginCreation(current);
    act(() => {
      current.store.getState().setActiveWorkspace(OTHER);
    });
    await finishCreation(current);
    expect(catalogue(current)).toMatchObject({ activeId: OTHER, activeIdRevision: 8 });
    expect(displayedNames()).toEqual([OTHER, ACCEPTED, BASE]);
    expect(catalogue(current).items.slice(1)).toEqual([descriptor(BASE), descriptor(OTHER)]);
  });

  it("doesNotCreateBlankName", async () => {
    const current = await mountCatalogue();
    const before = catalogue(current);
    openForm();
    fireEvent.change(screen.getByLabelText(NAME_LABEL), { target: { value: "   " } });
    fireEvent.submit(screen.getByLabelText(NAME_LABEL).closest("form")!);
    await act(async () => {});
    expect(current.tickets).toHaveLength(0);
    expect(catalogue(current)).toEqual(before);
    expect((screen.getByLabelText(NAME_LABEL) as HTMLInputElement).value).toBe("   ");
  });

  it("publishesOnlyToInitiatingProvider", async () => {
    const current = await mountCatalogue();
    const otherRoot = await mountCatalogue(
      [descriptor("separate-store")],
      "separate-store",
      19,
      false,
    );
    const separate = catalogue(otherRoot);
    expect(current.store).not.toBe(otherRoot.store);
    await beginCreation(current);
    notify(current, CREATED_EVENT, { id: INDEPENDENT, name: ADDED_NAME });
    await finishCreation(current);
    expect(catalogue(current).items[0].id).toBe(ACCEPTED);
    expect(catalogue(otherRoot)).toEqual(separate);
  });
});

// @covers AC-WORKSPACES-SETTINGS-UPDATES-003.5
it("rejectedCreationPreservesLiveCatalogueAndForm", async () => {
  const current = await mountCatalogue();
  await beginCreation(current, "Keep entered name");
  notify(current, CREATED_EVENT, { id: INDEPENDENT, name: ADDED_NAME });
  const before = catalogue(current);
  await openPicker();
  expectChoice(INDEPENDENT, ADDED_NAME);
  await finishCreation(current, descriptor(ACCEPTED), 409);
  expect(catalogue(current)).toEqual(before);
  expect(catalogue(current).items.some((row) => row.id === ACCEPTED)).toBe(false);
  expectChoice(INDEPENDENT, ADDED_NAME);
  expect((screen.getByLabelText(NAME_LABEL) as HTMLInputElement).value).toBe("Keep entered name");
  expect(
    screen.getAllByRole("button", { name: "Add Workspace" }).at(-1)?.hasAttribute("disabled"),
  ).toBe(false);
  expect(screen.getByText(REJECTION)).toBeDefined();
});
