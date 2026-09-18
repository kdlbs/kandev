import { act, renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import type { ImprovementDetail } from "@/lib/api/domains/assistant-maintenance-types";
import { useMaintenanceMutations } from "./use-maintenance-mutations";
const api = vi.hoisted(() => ({ run: vi.fn() }));
vi.mock("@/lib/api/domains/assistant-maintenance-api", () => ({ runMaintenance: api.run }));
const binding = {
  id: "binding",
  owner_user_id: "owner",
  version: 2,
  intent_revision: 3,
} as AssistantBinding;
const detail = {
  candidate: { id: "proposal", revision: 4 },
  grant: { revision: 5 },
} as ImprovementDetail;
beforeEach(() => {
  api.run.mockReset();
});
it("keeps an identical operation after a lost acknowledgement and prevents double clicks", async () => {
  api.run
    .mockRejectedValueOnce(new Error("lost acknowledgement"))
    .mockResolvedValueOnce({ commit_oid: "local" });
  const { result, rerender } = renderHook(
    ({ intent }) =>
      useMaintenanceMutations({ ...binding, intent_revision: intent }, detail, vi.fn()),
    { initialProps: { intent: 3 } },
  );
  await act(async () => {
    await Promise.all([result.current.maintenance("commit"), result.current.maintenance("commit")]);
  });
  expect(api.run).toHaveBeenCalledTimes(1);
  rerender({ intent: 4 });
  await act(async () => {
    await result.current.maintenance("commit");
  });
  expect(api.run.mock.calls[1]).toEqual(api.run.mock.calls[0]);
  expect(api.run.mock.calls[0][1]).toMatchObject({
    expected_intent_revision: 3,
    expected_binding_version: 2,
    candidate_revision: 4,
    grant_revision: 5,
  });
});
it("does not refresh a new owner view after an old action finishes", async () => {
  let finish!: (value: unknown) => void;
  api.run.mockImplementation(
    () =>
      new Promise((resolve) => {
        finish = resolve;
      }),
  );
  const refresh = vi.fn();
  const { result, rerender } = renderHook(
    ({ owner }) => useMaintenanceMutations({ ...binding, owner_user_id: owner }, detail, refresh),
    { initialProps: { owner: "owner" } },
  );
  let pending!: Promise<boolean>;
  act(() => {
    pending = result.current.maintenance("check");
  });
  rerender({ owner: "another-owner" });
  expect(result.current.busy).toBe(false);
  await act(async () => {
    finish({ passed: true });
    await pending;
  });
  expect(refresh).not.toHaveBeenCalled();
});
