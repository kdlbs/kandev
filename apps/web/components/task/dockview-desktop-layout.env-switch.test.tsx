import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render } from "@testing-library/react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import type { AppState } from "@/lib/state/store";
import type { TaskSession } from "@/lib/types/http";
import type { StoreApi } from "zustand";
import * as dockviewStore from "@/lib/state/dockview-store";
import { useEnvSwitchCleanup } from "./dockview-desktop-layout";

const OLD_TASK_ID = "task-old";
const NEW_TASK_ID = "task-new";
const NEW_SESSION_ID = "session-new";
const UNMAPPED_SESSION_ID = "session-unmapped";
let appStore: StoreApi<AppState> | null = null;

function Harness({ taskId, envId }: { taskId: string; envId: string }) {
  appStore = useAppStoreApi();
  useEnvSwitchCleanup(`session-${taskId}`, envId, taskId);
  return null;
}

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  appStore = null;
});

describe("useEnvSwitchCleanup", () => {
  it("keeps sessions with empty environment mappings unknown until hydration", () => {
    const switchLayout = vi
      .spyOn(dockviewStore, "performLayoutSwitch")
      .mockImplementation(() => undefined);
    const initialState = {
      ...defaultState,
      taskSessionsByTask: {
        ...defaultState.taskSessionsByTask,
        itemsByTaskId: {
          [NEW_TASK_ID]: [{ id: NEW_SESSION_ID } as TaskSession],
        },
        loadedByTaskId: { [NEW_TASK_ID]: false },
      },
    };

    const view = render(
      <StateProvider initialState={initialState}>
        <Harness taskId={OLD_TASK_ID} envId="env-old" />
      </StateProvider>,
    );
    act(() => {
      appStore?.setState({ environmentIdBySessionId: { [UNMAPPED_SESSION_ID]: "" } });
    });
    view.rerender(
      <StateProvider initialState={initialState}>
        <Harness taskId={NEW_TASK_ID} envId="env-new" />
      </StateProvider>,
    );

    expect(switchLayout).toHaveBeenCalledTimes(1);
    const options = switchLayout.mock.calls[0]?.[4];
    expect(options?.sessionListRestoreState?.knownForeignSessionIds).toEqual(new Set());
  });
});
