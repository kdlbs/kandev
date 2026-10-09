import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  ACCEPTED,
  OTHER,
  OWNER,
  TARGET,
  accept,
  cleanFixtures,
  deleted,
  executor,
  expectCatalogue,
  expectRejection,
  expectSuccess,
  initialCatalogue,
  mountCreation,
  profile,
  profileEvent,
  publish,
  reject,
  startCreation,
} from "./create-profile-catalogue-publication.test-helpers";

vi.mock("@/lib/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/client")>()),
  fetchJson: vi.fn(),
}));

// The external editor renderer is unavailable in the DOM test environment.
vi.mock("@monaco-editor/react", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@monaco-editor/react")>()),
  default: ({ value, options }: { value: string; options?: { ariaLabel?: string } }) => (
    <textarea aria-label={options?.ariaLabel} value={value} readOnly />
  ),
}));

beforeEach(() => vi.useFakeTimers());
afterEach(cleanFixtures);

const CREATED_EVENT = "executor.profile.created";
const LIVE_CHOICE = "Live choice";

describe("normal creation catalogue publication", () => {
  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.18
  it("unchanged catalogue creation submits payload and opens accepted profile", async () => {
    const fixture = await mountCreation();
    await startCreation(fixture);
    await accept(fixture);
    expectCatalogue(fixture, initialCatalogue());
    expectSuccess(fixture);
  });

  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.18
  it("unchanged catalogue rejection retains draft and route", async () => {
    const fixture = await mountCreation();
    await startCreation(fixture);
    await reject(fixture);
    expectCatalogue(fixture, initialCatalogue(), false);
    expectRejection(fixture);
  });
});

describe("current creation catalogue preservation", () => {
  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.16
  it("accepted creation retains another owners live profile and choice", async () => {
    const fixture = await mountCreation();
    await startCreation(fixture);
    profileEvent(fixture, CREATED_EVENT, profile("independent", OTHER, LIVE_CHOICE));
    const current = fixture.store.getState().executors.items;
    expect(current.find((owner) => owner.id === OTHER)?.profiles?.map((p) => p.id)).toEqual([
      "local-keep",
      "local-remove",
      "independent",
    ]);
    expect(fixture.options.find((option) => option.value === "independent")).toMatchObject({
      label: LIVE_CHOICE,
      disabled: false,
    });
    await accept(fixture);
    expectCatalogue(fixture, current);
    expectSuccess(fixture);
  });

  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.16
  it("accepted creation retains mixed different owner changes", async () => {
    const fixture = await mountCreation();
    await startCreation(fixture);
    profileEvent(fixture, CREATED_EVENT, profile("local-live", OTHER));
    profileEvent(
      fixture,
      "executor.profile.updated",
      profile("local-keep", OTHER, "Current local"),
    );
    deleted(fixture, "local-remove");
    publish(
      fixture,
      fixture.store
        .getState()
        .executors.items.filter((owner) => owner.id !== "removed-owner")
        .map((owner) =>
          owner.id === OTHER
            ? { ...owner, name: "Current local owner", config: { current: "local" } }
            : owner,
        )
        .concat(executor("live-owner", [profile("live-owner-profile", "live-owner")])),
    );
    const current = fixture.store.getState().executors.items;
    await accept(fixture);
    expectCatalogue(fixture, current);
    expectSuccess(fixture);
  });

  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.16
  it("accepted creation retains current owner metadata and siblings", async () => {
    const fixture = await mountCreation();
    await startCreation(fixture);
    profileEvent(fixture, CREATED_EVENT, profile("worktree-live"));
    profileEvent(
      fixture,
      "executor.profile.updated",
      profile("worktree-keep", OWNER, "Current sibling"),
    );
    deleted(fixture, "worktree-remove");
    publish(
      fixture,
      fixture.store.getState().executors.items.map((owner) =>
        owner.id === OWNER
          ? {
              ...owner,
              name: "Current worktree",
              status: "connected",
              config: { current: "owner" },
            }
          : owner,
      ),
    );
    const current = fixture.store.getState().executors.items;
    await accept(fixture);
    expectCatalogue(fixture, current);
    expectSuccess(fixture);
  });
});

describe("creation acknowledgement ordering", () => {
  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.17
  it("notification before response leaves one accepted target and choice", async () => {
    const fixture = await mountCreation();
    await startCreation(fixture);
    profileEvent(fixture, CREATED_EVENT, { ...ACCEPTED, name: "Notification target" });
    profileEvent(fixture, CREATED_EVENT, profile("independent", OTHER, LIVE_CHOICE));
    const current = fixture.store.getState().executors.items;
    expect(fixture.options.filter((option) => option.value === TARGET)).toHaveLength(1);
    await accept(fixture);
    expectCatalogue(fixture, current);
    expect(
      fixture.store
        .getState()
        .executors.items.find((owner) => owner.id === OWNER)
        ?.profiles?.filter((entry) => entry.id === TARGET),
    ).toEqual([ACCEPTED]);
    expect(fixture.options.filter((option) => option.value === TARGET)).toMatchObject([
      { value: TARGET, label: ACCEPTED.name, disabled: false },
    ]);
    expectSuccess(fixture);
  });

  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.18
  it("rejected creation keeps live catalogue and unsaved draft", async () => {
    const fixture = await mountCreation();
    await startCreation(fixture);
    profileEvent(fixture, CREATED_EVENT, profile("independent", OTHER, LIVE_CHOICE));
    const current = fixture.store.getState().executors.items;
    await reject(fixture);
    expectCatalogue(fixture, current, false);
    expectRejection(fixture);
  });

  // @covers AC-EXECUTORS-PROFILE-EDITOR-001.18
  it("creation does not restore an owner removed during transport", async () => {
    const fixture = await mountCreation();
    await startCreation(fixture);
    const current = fixture.store.getState().executors.items.filter((owner) => owner.id !== OWNER);
    publish(fixture, current);
    await accept(fixture);
    expectCatalogue(fixture, current, false);
    expectSuccess(fixture);
  });
});
