import { act, fireEvent, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  POLICY,
  OWNER,
  OTHER,
  deleted,
  event,
  executor,
  profile,
  publish,
  accepted,
  cleanFixtures,
  edit,
  expectCatalogue,
  expectClean,
  mountPolicy,
  policyInput,
  prepareTransport,
  settle,
  startSave,
  withAccepted,
  type Transport,
} from "./executor-policy-catalogue-publication.test-helpers";

// The external renderer lacks its browser capability here. Keep loader exports.
vi.mock("@monaco-editor/react", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@monaco-editor/react")>()),
  default: ({ value }: { value: string }) => <textarea value={value} readOnly />,
}));

const CREATED = "executor.profile.created";
const NEW_CHOICE = "new-choice";

beforeEach(prepareTransport);
afterEach(cleanFixtures);

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.20
describe("executor policy publication controls", () => {
  for (const transport of ["rest", "ws"] as const) {
    for (const isSystem of [true, false]) {
      it(`unchanged catalogue ${transport} success submits ${isSystem ? "system" : "custom"} policy`, async () => {
        const fixture = mountPolicy(transport, isSystem);
        await startSave(fixture);
        await settle(fixture);
        expectCatalogue(fixture, withAccepted(fixture, fixture.seed));
        expectClean(fixture);
        expect(policyInput(fixture).value).toBe(POLICY);
      });
    }

    it(`unchanged catalogue ${transport} failure retains draft and dirty state`, async () => {
      const fixture = mountPolicy(transport);
      await startSave(fixture);
      await settle(fixture, new Error("Network rejected save"));
      expectCatalogue(fixture, fixture.seed);
      expect(fixture.result?.canLeave).toBe(false);
      expect(fixture.coordinator).toMatchObject({
        hasDirty: true,
        status: "error",
        errorKind: "save",
      });
      expect(fixture.coordinator.contributorStates).toMatchObject([
        { id: "executor:policy-owner", isDirty: true, saveFailed: true },
      ]);
      expect(policyInput(fixture).value).toBe(POLICY);
    });

    it(`${transport} normalized response retains raw draft and sets accepted discard baseline`, async () => {
      const fixture = mountPolicy(transport);
      await startSave(fixture);
      const normalized = '{ "allow_http": true }';
      await settle(fixture, accepted(fixture, normalized));
      expectCatalogue(fixture, withAccepted(fixture, fixture.seed, normalized));
      expect(policyInput(fixture).value).toBe(POLICY);
      expect(fixture.coordinator.hasDirty).toBe(true);
      await act(async () => {
        fireEvent.click(within(fixture.view.container).getByRole("button", { name: "Reset" }));
      });
      expect(policyInput(fixture).value).toBe(normalized);
      expect(fixture.coordinator.hasDirty).toBe(false);
    });

    it(`${transport} acknowledges captured submission while preserving newer draft`, async () => {
      const fixture = mountPolicy(transport);
      await startSave(fixture);
      const newer = '{"allow_stdio":true}';
      edit(fixture, newer);
      await settle(fixture);
      expectCatalogue(fixture, withAccepted(fixture, fixture.seed));
      expect(fixture.payload?.config.mcp_policy).toBe(POLICY);
      expect(policyInput(fixture).value).toBe(newer);
      expect(fixture.result?.canLeave).toBe(false);
      expect(fixture.coordinator).toMatchObject({ hasDirty: true, status: "dirty" });
    });
  }
});

// @covers AC-EXECUTORS-PROFILE-EDITOR-001.19
// @covers AC-EXECUTORS-PROFILE-EDITOR-001.20
describe("executor policy catalogue preservation", () => {
  for (const transport of ["rest", "ws"] as const) {
    differentOwnerCases(transport);
    savedOwnerCases(transport);
  }

  it("isolates acknowledgements between live providers with equal owner IDs", async () => {
    const left = mountPolicy();
    const right = mountPolicy();
    expect(left.store).not.toBe(right.store);
    await startSave(left);
    await startSave(right, '{"allow_stdio":false}');
    event(left, CREATED, profile("left-live"));
    event(right, CREATED, profile("right-live"));
    deleted(right, "remove");
    const leftCurrent = left.store.getState().executors.items;
    const rightCurrent = right.store.getState().executors.items;
    await settle(left);
    expectCatalogue(left, withAccepted(left, leftCurrent));
    expectCatalogue(right, rightCurrent);
    expect(right.coordinator.status).toBe("saving");
    await settle(right);
    expectCatalogue(right, withAccepted(right, rightCurrent));
    expectCatalogue(left, withAccepted(left, leftCurrent));
    expectClean(left);
    expectClean(right);
  });
});

function differentOwnerCases(transport: Transport) {
  it(`${transport} retains another executor live created profile and usable choice`, async () => {
    const fixture = mountPolicy(transport);
    await startSave(fixture);
    event(fixture, CREATED, profile(NEW_CHOICE));
    const current = fixture.store.getState().executors.items;
    expectCatalogue(fixture, current);
    expect(fixture.options.find((item) => item.value === NEW_CHOICE)).toMatchObject({
      disabled: false,
    });
    await settle(fixture);
    expectCatalogue(fixture, withAccepted(fixture, current));
    expectClean(fixture);
  });

  it(`${transport} retains another executor updated profile and current option metadata`, async () => {
    const fixture = mountPolicy(transport);
    await startSave(fixture);
    event(fixture, "executor.profile.updated", profile("keep", OTHER, "Renamed choice"));
    publish(
      fixture,
      fixture.store
        .getState()
        .executors.items.map((owner) =>
          owner.id === OTHER
            ? { ...owner, name: "Renamed executor", config: { live: "current" } }
            : owner,
        ),
    );
    const current = fixture.store.getState().executors.items;
    expect(fixture.options.find((item) => item.value === "keep")).toMatchObject({
      label: "Renamed choice",
      executorName: "Renamed executor",
      disabled: false,
    });
    await settle(fixture);
    expectCatalogue(fixture, withAccepted(fixture, current));
    expectClean(fixture);
  });

  it(`${transport} keeps another executor deleted profile absent from usable choices`, async () => {
    const fixture = mountPolicy(transport);
    await startSave(fixture);
    deleted(fixture, "remove");
    const current = fixture.store.getState().executors.items;
    expect(fixture.options.some((item) => item.value === "remove")).toBe(false);
    await settle(fixture);
    expectCatalogue(fixture, withAccepted(fixture, current));
    expectClean(fixture);
  });

  it(`${transport} retains inserted owners and keeps removed owners absent in a mixed catalogue`, async () => {
    const fixture = mountPolicy(transport);
    publish(fixture, [
      ...fixture.seed,
      executor("retired", [profile("retired-choice", "retired")]),
    ]);
    await startSave(fixture);
    event(fixture, CREATED, profile(NEW_CHOICE));
    event(fixture, "executor.profile.updated", profile("keep", OTHER, "Current choice"));
    deleted(fixture, "remove");
    publish(fixture, [
      ...fixture.store.getState().executors.items.filter((owner) => owner.id !== "retired"),
      executor("inserted", [profile("inserted-choice", "inserted")]),
    ]);
    const current = fixture.store.getState().executors.items;
    expectCatalogue(fixture, current);
    expect(fixture.options.map((item) => item.value)).toEqual([
      "own-profile",
      "keep",
      NEW_CHOICE,
      "inserted-choice",
    ]);
    await settle(fixture);
    expectCatalogue(fixture, withAccepted(fixture, current));
    expectClean(fixture);
  });
}

function savedOwnerCases(transport: Transport) {
  it(`${transport} retains current saved-owner profile membership when response omits profiles`, async () => {
    const fixture = mountPolicy(transport);
    await startSave(fixture);
    event(fixture, "executor.profile.updated", profile("own-profile", OWNER, "Updated own choice"));
    event(fixture, CREATED, profile("own-added", OWNER));
    const current = fixture.store.getState().executors.items;
    expect(accepted(fixture)).not.toHaveProperty("profiles");
    expectCatalogue(fixture, current);
    await settle(fixture);
    expectCatalogue(fixture, withAccepted(fixture, current));
    expectClean(fixture);
  });

  it(`${transport} does not resurrect a removed saved executor`, async () => {
    const fixture = mountPolicy(transport);
    await startSave(fixture);
    publish(
      fixture,
      fixture.store.getState().executors.items.filter((owner) => owner.id !== OWNER),
    );
    const current = fixture.store.getState().executors.items;
    expect(fixture.view.container.querySelector("#mcp-policy")).toBeNull();
    await settle(fixture);
    expectCatalogue(fixture, current);
    expect(fixture.options.some((item) => item.value === "own-profile")).toBe(false);
  });

  it(`${transport} failure retains live events and failed current draft`, async () => {
    const fixture = mountPolicy(transport);
    await startSave(fixture);
    event(fixture, CREATED, profile("failure-live"));
    deleted(fixture, "remove");
    const current = fixture.store.getState().executors.items;
    edit(fixture, '{"allow_sse":true}');
    await settle(fixture, new Error("Transport refused policy"));
    expectCatalogue(fixture, current);
    expect(policyInput(fixture).value).toBe('{"allow_sse":true}');
    expect(fixture.coordinator).toMatchObject({
      hasDirty: true,
      status: "error",
      errorKind: "save",
    });
    expect(fixture.result?.failedIds).toEqual(new Set([`executor:${OWNER}`]));
  });
}
