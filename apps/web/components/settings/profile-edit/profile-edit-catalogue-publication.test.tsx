import { screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  OWNER,
  TARGET,
  PROFILE_ROUTE,
  accept,
  acceptedProfile,
  addedCatalogue,
  cleanFixtures,
  expectPreserved,
  initialCatalogue,
  mixedCatalogue,
  mountProfile,
  publish,
  reject,
  removedCatalogue,
  startDelete,
  startSave,
  updatedCatalogue,
} from "./profile-edit-catalogue-publication.test-helpers";

const INITIAL_NAME = "Original profile";
const SAVED_NAME = "Accepted profile";
const PROFILE_NAMES = { initialName: INITIAL_NAME, savedName: SAVED_NAME };

vi.mock("@/lib/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/client")>()),
  fetchJson: vi.fn(),
}));

// Only Monaco's external visual renderer needs a DOM-environment substitute.
vi.mock("@monaco-editor/react", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@monaco-editor/react")>()),
  default: ({ value, options }: { value: string; options?: { ariaLabel?: string } }) => (
    <textarea aria-label={options?.ariaLabel} value={value} readOnly />
  ),
}));

beforeEach(() => vi.useFakeTimers());
afterEach(cleanFixtures);

describe("normal profile catalogue publication", () => {
  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.13
  it.each([
    ["later executor and profile additions", addedCatalogue],
    ["current executor metadata and sibling updates", updatedCatalogue],
    ["unrelated removals without restoration", removedCatalogue],
    ["a mixed current catalogue", mixedCatalogue],
  ])("save retains %s", async (_name, laterCatalogue) => {
    const fixture = await mountProfile("PATCH", PROFILE_NAMES);
    await startSave(fixture);
    const current = laterCatalogue(INITIAL_NAME);
    publish(fixture, current);
    await accept(fixture, acceptedProfile(SAVED_NAME));

    expectPreserved(fixture, current);
    expect(
      fixture.store
        .getState()
        .executors.items.find((item) => item.id === OWNER)
        ?.profiles?.find((entry) => entry.id === TARGET),
    ).toEqual(acceptedProfile(SAVED_NAME));
    expect(fixture.coordinator.hasDirty).toBe(false);
  });

  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.5
  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.14
  it("unchanged catalogue save accepts target and clears dirty", async () => {
    const fixture = await mountProfile("PATCH", PROFILE_NAMES);
    await startSave(fixture);
    await accept(fixture, acceptedProfile(SAVED_NAME));

    expectPreserved(fixture, initialCatalogue(INITIAL_NAME));
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe(SAVED_NAME);
    expect(fixture.coordinator.hasDirty).toBe(false);
    expect(fixture.coordinator.status).toBe("saved");
    expect(within(screen.getByTestId("toast-container")).getByText("Profile saved")).toBeTruthy();
    expect(window.location.pathname).toBe(PROFILE_ROUTE);
  });

  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.14
  it("rejected save preserves current catalogue and dirty draft", async () => {
    const fixture = await mountProfile("PATCH", PROFILE_NAMES);
    await startSave(fixture);
    const current = mixedCatalogue(INITIAL_NAME);
    publish(fixture, current);
    await reject(fixture);

    expect(fixture.store.getState().executors.items).toEqual(current);
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe(SAVED_NAME);
    expect(fixture.coordinator.hasDirty).toBe(true);
    expect(fixture.coordinator.status).toBe("error");
    expect(fixture.coordinator.errorKind).toBe("save");
    const notifications = within(screen.getByTestId("toast-container"));
    expect(notifications.getByText("Failed to save profile")).toBeTruthy();
    expect(notifications.queryByText("Profile saved")).toBeNull();
    expect(window.location.pathname).toBe(PROFILE_ROUTE);
  });

  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.15
  it("delete retains mixed current catalogue and task options", async () => {
    const fixture = await mountProfile("DELETE", PROFILE_NAMES);
    await startDelete(fixture);
    const current = mixedCatalogue(INITIAL_NAME);
    publish(fixture, current);
    await accept(fixture, { success: true });

    expectPreserved(fixture, current, true);
    expect(fixture.options.some((item) => item.value === TARGET)).toBe(false);
    expect(window.location.pathname).toBe("/settings/executors");
  });

  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.15
  it("unchanged catalogue delete removes only target", async () => {
    const fixture = await mountProfile("DELETE", PROFILE_NAMES);
    await startDelete(fixture);
    await accept(fixture, { success: true });

    expectPreserved(fixture, initialCatalogue(INITIAL_NAME), true);
    expect(window.location.pathname).toBe("/settings/executors");
  });

  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.15
  it("rejected delete leaves current catalogue and route", async () => {
    const fixture = await mountProfile("DELETE", PROFILE_NAMES);
    await startDelete(fixture);
    const current = mixedCatalogue(INITIAL_NAME);
    publish(fixture, current);
    await reject(fixture);

    expect(fixture.store.getState().executors.items).toEqual(current);
    expect(window.location.pathname).toBe(PROFILE_ROUTE);
    expect(screen.queryByRole("dialog")).toBeNull();
    expect(screen.getByRole("button", { name: "Delete Profile" }).hasAttribute("disabled")).toBe(
      false,
    );
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe(INITIAL_NAME);
  });
});
