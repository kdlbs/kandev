import { expect, it, vi } from "vitest";
import {
  AssistantObservation,
  AssistantCollection,
} from "@/lib/orchestration/assistant-observation";
import { ApiError } from "@/lib/api/client";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
const binding = {
  id: "binding",
  owner_user_id: "owner",
  version: 1,
  home_workspace_id: "home",
  conversation_id: "private",
  orchestrator_id: "assistant",
  execution_mode: "inspect",
  intent_revision: 0,
  paused: false,
} satisfies AssistantBinding;
it("clears private state and aborts delayed reads on owner changes", async () => {
  let finish!: (value: AssistantBinding) => void;
  const load = vi.fn().mockImplementation(
    () =>
      new Promise<AssistantBinding>((resolve) => {
        finish = resolve;
      }),
  );
  const old = new AssistantObservation(load);
  const request = old.refresh();
  old.dispose();
  expect(load.mock.calls[0][0].aborted).toBe(true);
  const next = new AssistantObservation(vi.fn().mockResolvedValue(null));
  await next.refresh();
  finish(binding);
  await request;
  expect(old.getSnapshot().binding).toBeUndefined();
  expect(next.getSnapshot().binding).toBeNull();
});
it("a revision invalidation supersedes an older HTTP response", async () => {
  let finish!: (value: AssistantBinding) => void;
  const load = vi
    .fn()
    .mockImplementationOnce(
      () =>
        new Promise<AssistantBinding>((resolve) => {
          finish = resolve;
        }),
    )
    .mockResolvedValue({ ...binding, version: 2 });
  const view = new AssistantObservation(load);
  const older = view.refresh();
  await view.refresh();
  finish(binding);
  await older;
  expect(view.getSnapshot().binding?.version).toBe(2);
  view.dispose();
});
it("keeps a distinction between loading, unconfigured and unavailable", async () => {
  const load = vi
    .fn()
    .mockResolvedValueOnce(binding)
    .mockRejectedValueOnce(new Error("Offline"))
    .mockResolvedValue(null);
  const view = new AssistantObservation(load);
  expect(view.getSnapshot().binding).toBeUndefined();
  await view.refresh();
  await view.refresh();
  expect(view.getSnapshot().binding).toEqual(binding);
  expect(view.getSnapshot().error).toBeTruthy();
  await view.refresh();
  expect(view.getSnapshot().binding).toBeNull();
  expect(view.getSnapshot().error).toBeNull();
  view.dispose();
});
it("refreshes the loaded cursor window and removes denied or forgotten rows", async () => {
  const load = vi
    .fn()
    .mockResolvedValueOnce({ entries: [{ id: "one" }], next_cursor: "cursor" })
    .mockResolvedValueOnce({ entries: [{ id: "one" }], next_cursor: "cursor" })
    .mockResolvedValueOnce({ entries: [{ id: "two" }], next_cursor: "" })
    .mockResolvedValueOnce({ entries: [{ id: "two" }], next_cursor: "" })
    .mockRejectedValueOnce(new ApiError("Denied", 403, {}));
  const view = new AssistantCollection(load);
  await view.refresh();
  expect(view.getSnapshot().nextCursor).toBe("cursor");
  await view.loadMore();
  expect(view.getSnapshot().entries).toEqual([{ id: "one" }, { id: "two" }]);
  expect(load.mock.calls[2][0]).toBe("cursor");
  await view.refresh();
  expect(view.getSnapshot().entries).toEqual([{ id: "two" }]);
  await view.refresh();
  expect(view.getSnapshot().entries).toEqual([]);
  expect(view.getSnapshot().error).toBeTruthy();
  view.dispose();
});
