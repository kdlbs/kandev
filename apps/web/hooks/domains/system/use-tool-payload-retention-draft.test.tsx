import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { SettingsSaveContributor } from "@/components/settings/settings-save-provider";
import type { ToolPayloadRetentionStatus } from "@/lib/types/tool-payload-retention";
import { useToolPayloadRetentionDraft } from "./use-tool-payload-retention-draft";
import type { useToolPayloadRetention } from "./use-tool-payload-retention";

let contributor: SettingsSaveContributor | undefined;
vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveContributor: (next: SettingsSaveContributor) => {
    contributor = next;
  },
}));

const status: ToolPayloadRetentionStatus = {
  supported: true,
  policy: { enabled: false, age: { value: 3, unit: "months" }, revision: 1 },
  preparation: { state: "none", choice: "" },
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

afterEach(() => {
  cleanup();
  contributor = undefined;
});

it("does not apply an old save response to the current draft or clear its backup choice", async () => {
  let current = true;
  const response = deferred<ToolPayloadRetentionStatus>();
  const scope = { identityKey: "backend-a-user-1", generation: 0 };
  const remote = {
    status,
    pending: false,
    captureScope: vi.fn(() => scope),
    isCurrentScope: vi.fn(() => current),
    save: vi.fn(() => response.promise),
    refresh: vi.fn(),
  } as unknown as ReturnType<typeof useToolPayloadRetention>;
  const { result } = renderHook(() => useToolPayloadRetentionDraft(remote, true));
  act(() => {
    result.current.setEnabled(true);
    result.current.setChoice("backup");
  });
  if (!contributor) throw new Error("The retention save contributor should be registered");

  let save!: Promise<void>;
  act(() => {
    save = Promise.resolve(contributor?.save(contributor.revision));
  });
  expect(remote.save).toHaveBeenCalledWith(
    { ...status.policy, enabled: true, backup_choice: "backup" },
    { backupChoiceAttempt: true },
  );

  current = false;
  await act(async () => {
    response.resolve({
      ...status,
      policy: { ...status.policy, enabled: true, revision: 2 },
      preparation: { state: "ready", choice: "backup" },
    });
    await save;
  });

  expect(result.current.choice).toBe("backup");
  expect(result.current.draft).toMatchObject({ enabled: true, revision: 1 });
});

it("resets a dirty draft and backup choice when the authenticated identity changes", async () => {
  const nextStatus: ToolPayloadRetentionStatus = {
    ...status,
    policy: { enabled: true, age: { value: 2, unit: "weeks" }, revision: 9 },
  };
  const makeRemote = (
    next: ToolPayloadRetentionStatus,
    identityKey: string,
  ): ReturnType<typeof useToolPayloadRetention> =>
    ({
      status: next,
      pending: false,
      captureScope: () => ({ identityKey, generation: 0 }),
      isCurrentScope: () => true,
      save: vi.fn(),
      refresh: vi.fn(),
    }) as unknown as ReturnType<typeof useToolPayloadRetention>;

  const remoteA = makeRemote(status, "backend-a-user-1");
  const { result, rerender } = renderHook(
    ({ remote }) => useToolPayloadRetentionDraft(remote, true),
    { initialProps: { remote: remoteA } },
  );
  await waitFor(() => expect(result.current.draft).toEqual(status.policy));
  act(() => {
    result.current.setEnabled(true);
    result.current.setChoice("backup");
  });
  expect(result.current.dirty).toBe(true);

  rerender({ remote: makeRemote(nextStatus, "backend-b-user-2") });

  await waitFor(() => expect(result.current.draft).toEqual(nextStatus.policy));
  expect(result.current.choice).toBe("");
});
