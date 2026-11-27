import { afterEach, beforeEach, describe, expect, it } from "vitest";
import {
  CREATED,
  LIVE,
  REMOVED,
  SURVIVOR,
  expectInventory,
  finish,
  mount,
  ownerPayload,
  prepare,
  profile,
  publishOwner,
  publishProfile,
  removeProfile,
  settle,
  start,
  type Transport,
} from "./executor-create-catalogue-publication.test-helpers";

beforeEach(prepare);
afterEach(finish);

describe.each<Transport>(["http", "ws"])("accepted executor creation (%s)", (transport) => {
  // @covers AC-EXECUTORS-CREATE-CATALOGUE-001.2, .3, .4
  it("ordinary accepted creation retains payload and choices", async () => {
    const fixture = mount(transport);
    await start(fixture);
    await expectInventory(fixture, fixture.seed);
    await settle(fixture);
    await expectInventory(fixture, [...fixture.seed, fixture.accepted]);
    expect(
      fixture.store.getState().executors.items.filter((entry) => entry.id === CREATED),
    ).toEqual([fixture.accepted]);
    expect(fixture.accepted.profiles).toBeUndefined();
  });

  // @covers AC-EXECUTORS-CREATE-CATALOGUE-001.1, .3, .4
  it("accepted creation preserves mixed live catalogue changes", async () => {
    const fixture = mount(transport);
    await start(fixture);
    publishOwner(fixture, "executor.created", ownerPayload(LIVE));
    publishProfile(fixture, "executor.profile.created", profile("live-choice", LIVE));
    publishOwner(fixture, "executor.deleted", ownerPayload(REMOVED));
    const updatedOwner = {
      ...ownerPayload(SURVIVOR),
      name: "Current survivor",
      config: { live: "survivor-config" },
    };
    const updatedProfile = {
      ...profile("survivor-choice", SURVIVOR),
      name: "Current survivor profile",
      prepare_script: "current prepare",
    };
    publishOwner(fixture, "executor.updated", updatedOwner);
    publishProfile(fixture, "executor.profile.updated", updatedProfile);
    removeProfile(fixture, "obsolete-profile");
    publishProfile(fixture, "executor.profile.created", profile("survivor-new-choice", SURVIVOR));
    const current = fixture.store.getState().executors.items;
    expect(current.map((entry) => entry.id)).toEqual([SURVIVOR, LIVE]);
    expect(current[0]).toMatchObject({ name: updatedOwner.name, config: updatedOwner.config });
    expect(current[0].profiles?.map((entry) => entry.id)).toEqual([
      "survivor-choice",
      "survivor-new-choice",
    ]);
    expect(current[0].profiles?.[0]).toMatchObject(updatedProfile);
    await expectInventory(fixture, current);
    await settle(fixture);
    await expectInventory(fixture, [...current, fixture.accepted]);
    expect(
      fixture.store.getState().executors.items.filter((entry) => entry.id === CREATED),
    ).toEqual([fixture.accepted]);
  });

  // @covers AC-EXECUTORS-CREATE-CATALOGUE-001.2, .4
  it("owner notification before acceptance keeps the accepted descriptor once and last", async () => {
    const fixture = mount(transport);
    await start(fixture);
    publishOwner(fixture, "executor.created", {
      ...fixture.accepted,
      name: "Notification descriptor",
      config: { event: "owner-only" },
    });
    expect(fixture.store.getState().executors.items.at(-1)?.name).toBe("Notification descriptor");
    await settle(fixture);
    await expectInventory(fixture, [...fixture.seed, fixture.accepted]);
    expect(
      fixture.store.getState().executors.items.filter((entry) => entry.id === CREATED),
    ).toEqual([fixture.accepted]);
    expect(fixture.store.getState().executors.items.at(-1)).toEqual(fixture.accepted);
  });
});
