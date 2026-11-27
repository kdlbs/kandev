import { act, fireEvent, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {
  executor,
  profile,
  transport,
  mountCard,
  startDelete,
  releaseRead,
  publish,
  createOwner,
  disposeFixtures,
  TARGET,
  SIBLING,
  holdDeletion,
  json,
} from "./executor-profiles-card-refresh.test-helpers";

const SAVED_SIBLING = "Saved sibling";
const SIBLING_CURRENT = "sibling-current";
const SIBLING_CHOICE = "choice-sibling-current";
const ACCEPTED = "accepted-target";
const ACCEPTED_CHOICE = "choice-accepted-target";
const NEW_OWNER = "new-owner";
const NEW_OWNER_CHOICE = "choice-new-owner-profile";
const STALE = "stale-response";
const STALE_CHOICE = "choice-stale-response";

const PROFILE_CREATED = "executor.profile.created";
const CURRENT_TARGET = "Current target";
const SECOND_TARGET = "second-target";

afterEach(disposeFixtures);

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.25 through .27
describe("executor card refresh publication", () => {
  it("retains a saved sibling name after pending refresh", async () => {
    const fixture = transport();
    const mount = mountCard();
    const read = await startDelete(mount, fixture);
    const updated = executor(SIBLING);
    delete updated.profiles;
    publish(mount.store, "executor.updated", { ...updated, name: SAVED_SIBLING });
    // Executor update DTOs omit profiles; retain real registered merge semantics.
    publish(mount.store, PROFILE_CREATED, profile(SIBLING_CURRENT, SIBLING));
    expect(mount.view.getByTestId(SIBLING_CHOICE).textContent).toContain(SAVED_SIBLING);
    await releaseRead(read, [profile(ACCEPTED)]);
    expect
      .soft(mount.store.getState().executors.items.find((item) => item.id === SIBLING)?.name)
      .toBe(SAVED_SIBLING);
    expect
      .soft(mount.view.queryByTestId(SIBLING_CHOICE)?.textContent ?? "")
      .toContain(SAVED_SIBLING);
    expect(mount.view.getByTestId(ACCEPTED_CHOICE)).toBeTruthy();
  });

  it("retains an executor created during pending refresh", async () => {
    const fixture = transport();
    const mount = mountCard();
    const read = await startDelete(mount, fixture);
    createOwner(mount.store, NEW_OWNER);
    expect(mount.view.getByTestId(NEW_OWNER_CHOICE).getAttribute("data-disabled")).toBe("false");
    await releaseRead(read, [profile(ACCEPTED)]);
    expect.soft(mount.store.getState().executors.items.map((item) => item.id)).toContain(NEW_OWNER);
    expect.soft(mount.view.queryByTestId(NEW_OWNER_CHOICE)?.textContent ?? "").toContain(NEW_OWNER);
    expect(mount.view.getByTestId(ACCEPTED_CHOICE)).toBeTruthy();
  });

  it("publishes an ordinary target refresh", async () => {
    const fixture = transport();
    const mount = mountCard();
    const sibling = mount.store.getState().executors.items[1];
    const read = await startDelete(mount, fixture);
    await releaseRead(read, [profile(ACCEPTED)]);
    expect(mount.view.queryByTestId(`choice-${TARGET}-profile`)).toBeNull();
    expect(mount.view.getByTestId(ACCEPTED_CHOICE).textContent).toContain(TARGET);
    expect(mount.store.getState().executors.items[1]).toBe(sibling);
    expect(fixture.deletes).toEqual([`${TARGET}-profile`]);
  });

  it("retains current inventory after GET failure", async () => {
    const fixture = transport();
    const mount = mountCard();
    const read = await startDelete(mount, fixture);
    createOwner(mount.store, NEW_OWNER);
    const current = mount.store.getState().executors.items;
    expect(mount.view.getByTestId(NEW_OWNER_CHOICE)).toBeTruthy();
    await releaseRead(read, [], true);
    expect(mount.store.getState().executors.items).toBe(current);
    expect(mount.view.getByTestId(NEW_OWNER_CHOICE)).toBeTruthy();
  });
});

describe("executor card refresh publication: unrelated inventory", () => {
  it("keeps unrelated removals absent in a mixed catalogue", async () => {
    const fixture = transport();
    const mount = mountCard([executor(), executor(SIBLING), executor("removed-owner")]);
    const read = await startDelete(mount, fixture);
    publish(mount.store, "executor.deleted", executor("removed-owner"));
    publish(mount.store, "executor.profile.deleted", { id: `${SIBLING}-profile` });
    createOwner(mount.store, NEW_OWNER);
    publish(mount.store, PROFILE_CREATED, {
      ...profile("new-sibling", SIBLING),
      name: "Current sibling",
    });
    const current = mount.store.getState().executors.items;
    expect(mount.view.queryByTestId("choice-removed-owner-profile")).toBeNull();
    expect(mount.view.getByTestId("choice-new-sibling").textContent).toContain("Current sibling");
    await releaseRead(read, [profile(ACCEPTED)]);
    expect(mount.store.getState().executors.items.map((item) => item.id)).toEqual([
      TARGET,
      SIBLING,
      NEW_OWNER,
    ]);
    expect(mount.store.getState().executors.items.slice(1)).toEqual(current.slice(1));
    expect(mount.view.queryByTestId(`choice-${SIBLING}-profile`)).toBeNull();
    expect(mount.view.queryByTestId("choice-removed-owner-profile")).toBeNull();
    expect(mount.view.getByTestId(NEW_OWNER_CHOICE)).toBeTruthy();
    expect(mount.view.getByTestId("choice-new-sibling").textContent).toContain("Current sibling");
  });

  it("retains target metadata while refreshing its profiles", async () => {
    const fixture = transport();
    const mount = mountCard();
    const read = await startDelete(mount, fixture);
    const updated = executor();
    delete updated.profiles;
    publish(mount.store, "executor.updated", {
      ...updated,
      name: CURRENT_TARGET,
      config: { retained: "current" },
      status: "inactive",
    });
    expect(mount.view.getByTestId(`choice-${TARGET}-profile`).textContent).toContain(
      CURRENT_TARGET,
    );
    await releaseRead(read, [profile(ACCEPTED)]);
    expect(mount.store.getState().executors.items[0]).toMatchObject({
      name: CURRENT_TARGET,
      config: { retained: "current" },
      status: "inactive",
    });
    expect(mount.view.getByTestId(ACCEPTED_CHOICE).textContent).toContain(CURRENT_TARGET);
  });

  it("publishes an empty returned profile list", async () => {
    const fixture = transport();
    const mount = mountCard();
    await releaseRead(await startDelete(mount, fixture), []);
    expect(mount.store.getState().executors.items[0].profiles).toEqual([]);
    expect(mount.view.getByTestId(`choice-${SIBLING}-profile`)).toBeTruthy();
    expect(mount.view.queryByTestId(`choice-${TARGET}-profile`)).toBeNull();
  });
});

describe("executor card refresh publication: target transitions", () => {
  it.each(["create", "update", "delete", "restore"] as const)(
    "retains the current target list after a live profile transition: %s",
    async (change) => {
      const fixture = transport();
      const mount = mountCard();
      const read = await startDelete(mount, fixture);
      if (change === "create") publish(mount.store, PROFILE_CREATED, profile("live-target"));
      if (change === "delete")
        publish(mount.store, "executor.profile.deleted", { id: `${TARGET}-profile` });
      if (change === "update" || change === "restore") {
        publish(mount.store, "executor.profile.updated", {
          ...profile(`${TARGET}-profile`),
          name: "Live target",
        });
        if (change === "restore")
          publish(mount.store, "executor.profile.updated", profile(`${TARGET}-profile`));
      }
      const current = mount.store.getState().executors.items[0].profiles;
      if (change === "create") expect(mount.view.getByTestId("choice-live-target")).toBeTruthy();
      if (change === "delete")
        expect(mount.view.queryByTestId(`choice-${TARGET}-profile`)).toBeNull();
      await releaseRead(read, [profile(STALE)]);
      expect(mount.store.getState().executors.items[0].profiles).toBe(current);
      expect(mount.view.queryByTestId(STALE_CHOICE)).toBeNull();
      if (change === "update")
        expect(mount.view.getByTestId(`choice-${TARGET}-profile`).textContent).toContain(
          "Live target",
        );
    },
  );

  it("accepts refresh after unrelated profile deletion clones", async () => {
    const fixture = transport();
    const mount = mountCard();
    const read = await startDelete(mount, fixture);
    const before = mount.store.getState().executors.items[0];
    publish(mount.store, "executor.profile.deleted", { id: `${SIBLING}-profile` });
    const cloned = mount.store.getState().executors.items[0];
    expect(cloned).not.toBe(before);
    expect(cloned.profiles).not.toBe(before.profiles);
    expect(cloned.profiles?.[0]).toBe(before.profiles?.[0]);
    await releaseRead(read, [profile(ACCEPTED)]);
    expect(mount.view.getByTestId(ACCEPTED_CHOICE)).toBeTruthy();
    expect(mount.view.queryByTestId(`choice-${SIBLING}-profile`)).toBeNull();
  });

  it.each([false, true])(
    "does not restore a missing target (reintroduced: %s)",
    async (reintroduced) => {
      const fixture = transport();
      const mount = mountCard();
      const read = await startDelete(mount, fixture);
      publish(mount.store, "executor.deleted", executor());
      if (reintroduced) createOwner(mount.store, TARGET);
      const current = mount.store.getState().executors.items;
      await releaseRead(read, [profile(STALE)]);
      expect(mount.store.getState().executors.items).toBe(current);
      expect(mount.view.queryByTestId(STALE_CHOICE)).toBeNull();
      expect(mount.view.getByTestId(`choice-${SIBLING}-profile`)).toBeTruthy();
    },
  );

  it("does not publish when target was absent at GET admission", async () => {
    const fixture = transport();
    const held = holdDeletion(fixture);
    const mount = mountCard();
    fireEvent.click(
      within(mount.view.getByTestId(`executor-profile-card-${TARGET}-profile`)).getByTestId(
        "executor-profile-delete-button",
      ),
    );
    expect(fixture.deletes).toHaveLength(1);
    publish(mount.store, "executor.deleted", executor());
    await act(async () => {
      held.resolve(json({ success: true }));
      await held.promise;
    });
    await waitFor(() => expect(fixture.reads).toHaveLength(1));
    createOwner(mount.store, TARGET);
    const current = mount.store.getState().executors.items;
    await releaseRead(fixture.reads[0], [profile(STALE)]);
    expect(mount.store.getState().executors.items).toBe(current);
    expect(mount.view.queryByTestId(STALE_CHOICE)).toBeNull();
  });
});

describe("executor card refresh publication: read ordering and navigation", () => {
  it("keeps the newer overlapping read result", async () => {
    const fixture = transport();
    const mount = mountCard();
    const older = await startDelete(mount, fixture);
    const newer = await startDelete(mount, fixture);
    await releaseRead(newer, [profile("newer-result")]);
    expect(mount.view.getByTestId("choice-newer-result")).toBeTruthy();
    await releaseRead(older, [profile("older-result")]);
    expect(mount.view.getByTestId("choice-newer-result")).toBeTruthy();
    expect(mount.view.queryByTestId("choice-older-result")).toBeNull();
  });

  it("discards the older read when the newer read fails", async () => {
    const fixture = transport();
    const mount = mountCard();
    const older = await startDelete(mount, fixture);
    const newer = await startDelete(mount, fixture);
    const current = mount.store.getState().executors.items;
    await releaseRead(newer, [], true);
    await releaseRead(older, [profile("older-result")]);
    expect(mount.store.getState().executors.items).toBe(current);
    expect(mount.view.queryByTestId("choice-older-result")).toBeNull();
    expect(mount.view.getByTestId(`choice-${TARGET}-profile`)).toBeTruthy();
  });

  it("does not refresh after failed deletion", async () => {
    const fixture = transport();
    fixture.deleteFailure = true;
    const mount = mountCard();
    const current = mount.store.getState().executors.items;
    await act(async () =>
      fireEvent.click(mount.view.getByTestId("executor-profile-delete-button")),
    );
    expect(fixture.deletes).toHaveLength(1);
    expect(fixture.reads).toHaveLength(0);
    expect(mount.store.getState().executors.items).toBe(current);
    expect(mount.view.getByTestId(`choice-${TARGET}-profile`)).toBeTruthy();
  });

  it.each(["success", "failure", "contested"] as const)(
    "navigates after creation refresh settles: %s",
    async (outcome) => {
      const fixture = transport();
      const mount = mountCard();
      fireEvent.click(mount.view.getByRole("button", { name: "Add" }));
      fireEvent.change(mount.view.getByLabelText("Name"), { target: { value: "Created profile" } });
      fireEvent.click(mount.view.getByRole("button", { name: "Create Profile" }));
      await waitFor(() => expect(fixture.reads).toHaveLength(1));
      expect(fixture.posts).toEqual([{ name: "Created profile" }]);
      expect(window.location.pathname).not.toBe("/settings/executors/created-via-dialog");
      if (outcome === "contested") publish(mount.store, PROFILE_CREATED, profile("live-target"));
      await releaseRead(fixture.reads[0], [profile("created-via-dialog")], outcome === "failure");
      await waitFor(() =>
        expect(window.location.pathname).toBe("/settings/executors/created-via-dialog"),
      );
      if (outcome === "success")
        expect(mount.view.getByTestId("choice-created-via-dialog")).toBeTruthy();
      if (outcome === "contested")
        expect(mount.view.getByTestId("choice-live-target")).toBeTruthy();
    },
  );

  it("isolates separate owning stores", async () => {
    const fixture = transport();
    const first = mountCard();
    const second = mountCard([executor(SECOND_TARGET)], SECOND_TARGET);
    const older = await startDelete(first, fixture);
    const newer = await startDelete(second, fixture, "second-target-profile");
    createOwner(first.store, "first-only");
    await releaseRead(newer, [profile("second-result", SECOND_TARGET)]);
    await releaseRead(older, [profile("first-result")]);
    expect(first.store.getState().executors.items.map((item) => item.id)).toEqual([
      TARGET,
      SIBLING,
      "first-only",
    ]);
    expect(second.store.getState().executors.items.map((item) => item.id)).toEqual([SECOND_TARGET]);
    expect(first.view.getByTestId("choice-first-result")).toBeTruthy();
    expect(second.view.getByTestId("choice-second-result")).toBeTruthy();
  });
});
