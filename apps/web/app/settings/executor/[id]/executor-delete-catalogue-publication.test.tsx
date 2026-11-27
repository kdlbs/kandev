import { fireEvent, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { deleteExecutorAction } from "@/app/actions/executors";
import {
  TARGET,
  RETAINED,
  accept,
  executor,
  expectInventory,
  mount,
  openConfirmation,
  ownerEvent,
  prepare,
  profile,
  profileEvent,
  requests,
  start,
  teardown,
} from "./executor-delete-catalogue-publication.test-helpers";

// Monaco's external browser renderer cannot run here; retain loader exports.
vi.mock("@monaco-editor/react", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@monaco-editor/react")>()),
  default: ({ value }: { value: string }) => <textarea readOnly value={value} />,
}));

const CREATED_PROFILE = "executor.profile.created";
const RETAINED_PROFILE = "retained-profile";

beforeEach(prepare);
afterEach(teardown);

// @covers AC-EXECUTORS-OWNER-DELETION-001.1
// @covers AC-EXECUTORS-OWNER-DELETION-001.2
// @covers AC-EXECUTORS-OWNER-DELETION-001.3
describe("executor owner deletion publication", () => {
  it("retains mixed live catalogue after accepted HTTP owner deletion", async () => {
    const fixture = mount();
    await start(fixture);
    ownerEvent(fixture, "executor.created", executor("new-owner"));
    profileEvent(fixture, CREATED_PROFILE, profile("created-choice", "new-owner"));
    ownerEvent(fixture, "executor.deleted", fixture.seed[2]);
    const current = fixture.store.getState().executors.items;
    expect(current.map((owner) => owner.id)).toEqual([TARGET, RETAINED, "new-owner"]);
    expect(fixture.options.map((item) => item.value)).toEqual([
      "target-profile",
      RETAINED_PROFILE,
      "created-choice",
    ]);
    await accept(fixture);
    await expectInventory(
      fixture,
      current.filter((owner) => owner.id !== TARGET),
    );
  });

  it("ordinary accepted HTTP owner deletion", async () => {
    const fixture = mount();
    await start(fixture);
    await accept(fixture);
    await expectInventory(fixture, fixture.seed.slice(1));
    expect(requests).toHaveLength(1);
  });

  it("mismatched exact deletion confirmation", () => {
    const fixture = mount();
    const { input, button } = openConfirmation();
    for (const value of ["", "Delete", "DELETE", " delete", "delete ", "remove"]) {
      fireEvent.change(input, { target: { value } });
      expect((button as HTMLButtonElement).disabled).toBe(true);
      fireEvent.click(button);
      expect(fixture.admitted).toBe(false);
      expect(requests).toHaveLength(0);
      expect(fixture.store.getState().executors.items).toEqual(fixture.seed);
      expect(window.location.pathname).toBe(`/settings/executor/${TARGET}`);
    }
    fireEvent.change(input, { target: { value: "delete" } });
    expect((button as HTMLButtonElement).disabled).toBe(false);
  });

  it("ordinary accepted WS owner deletion", async () => {
    const fixture = mount("WS");
    await start(fixture);
    await accept(fixture);
    await expectInventory(fixture, fixture.seed.slice(1));
    expect(requests).toHaveLength(0);
  });

  it("retains mixed live catalogue after accepted WS owner deletion", async () => {
    const fixture = mount("WS");
    await start(fixture);
    ownerEvent(fixture, "executor.deleted", fixture.seed[2]);
    ownerEvent(fixture, "executor.created", executor("ws-created"));
    profileEvent(fixture, CREATED_PROFILE, profile("ws-choice", "ws-created"));
    const current = fixture.store.getState().executors.items;
    expect(fixture.options.map((item) => item.value)).toEqual([
      "target-profile",
      RETAINED_PROFILE,
      "ws-choice",
    ]);
    await accept(fixture);
    await expectInventory(
      fixture,
      current.filter((owner) => owner.id !== TARGET),
    );
  });

  survivorCases();

  compatibilityControls();
});

function survivorCases() {
  for (const transport of ["HTTP", "WS"] as const) {
    it(`retains live survivor fields and profiles through ${transport}`, async () => {
      const fixture = mount(transport);
      await start(fixture);
      ownerEvent(fixture, "executor.updated", {
        ...fixture.seed[1],
        name: "Current owner",
        status: "inactive",
        config: { current: "changed" },
        updated_at: "2026-10-10T05:00:00Z",
      });
      profileEvent(
        fixture,
        "executor.profile.updated",
        profile(RETAINED_PROFILE, RETAINED, "Current profile"),
      );
      profileEvent(fixture, CREATED_PROFILE, profile("sibling-choice", RETAINED));
      profileEvent(fixture, "executor.profile.deleted", { id: "removed-profile" });
      const current = fixture.store.getState().executors.items;
      expect(fixture.options.find((item) => item.value === RETAINED_PROFILE)).toMatchObject({
        label: "Current profile",
        executorName: "Current owner",
        executorType: "local_pc",
        disabled: false,
      });
      await accept(fixture);
      await expectInventory(
        fixture,
        current.filter((owner) => owner.id !== TARGET),
      );
      expect(fixture.options.find((item) => item.value === RETAINED_PROFILE)).toMatchObject({
        label: "Current profile",
        executorName: "Current owner",
        disabled: false,
      });
    });
  }
}

function compatibilityControls() {
  it("target notification before acknowledgement", async () => {
    const fixture = mount();
    await start(fixture);
    ownerEvent(fixture, "executor.deleted", fixture.seed[0]);
    profileEvent(fixture, CREATED_PROFILE, profile("later-choice", RETAINED));
    const current = fixture.store.getState().executors.items;
    expect(screen.getByText("Executor not found")).not.toBeNull();
    await accept(fixture);
    await expectInventory(fixture, current);
  });

  it("system executor has no owner delete action", () => {
    const fixture = mount("HTTP", true);
    expect(screen.queryByText("Remove this executor")).toBeNull();
    expect(screen.queryByRole("heading", { name: "Delete Executor" })).toBeNull();
    expect(fixture.admitted).toBe(false);
    expect(requests).toHaveLength(0);
  });

  it("rejects HTTP owner deletion at the action boundary", async () => {
    const fetchMock = vi.mocked(fetch);
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 403, statusText: "Forbidden" }));
    // Observe the real adapter's rejection. React does not await its click callback.
    await expect(deleteExecutorAction(TARGET)).rejects.toThrow("Request failed: 403 Forbidden");
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining(`/api/v1/executors/${TARGET}`),
      expect.objectContaining({ method: "DELETE", cache: "no-store" }),
    );
  });
}
