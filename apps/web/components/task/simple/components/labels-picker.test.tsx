import { useEffect, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { TaskOptimisticContextProvider } from "@/hooks/use-optimistic-task-mutation";
import { __resetOfficeTaskContentSyncForTests } from "@/lib/state/office-task-content-sync";
import { toast } from "@/lib/toast/sonner";
import type { Task } from "@/app/office/tasks/[id]/types";
import { LabelsPicker } from "./labels-picker";

vi.mock("@/lib/toast/sonner", () => ({ toast: { error: vi.fn() } }));

const removeSlashLabelName = "Remove area/backend";

const seedTask: Task = {
  id: "label-task",
  workspaceId: "label-workspace",
  identifier: "LABEL-1",
  title: "Label transport",
  status: "todo",
  priority: "medium",
  labels: [
    { name: "area/backend", color: "#ff0000" },
    { name: "area%2Fbackend", color: "#00ff00" },
    { name: "urgent", color: "#0000ff" },
  ],
  blockedBy: [],
  blocking: [],
  children: [],
  reviewers: [],
  approvers: [],
  decisions: [],
  createdBy: "user",
  createdAt: "2026-05-01T00:00:00Z",
  updatedAt: "2026-05-01T00:00:00Z",
};

function renderPicker() {
  let store: ReturnType<typeof useAppStoreApi> | undefined;
  function Harness() {
    const [task, setTask] = useState(seedTask);
    const api = useAppStoreApi();
    useEffect(() => {
      store = api;
      api.getState().setTasks([{ ...seedTask, children: [] }]);
    }, [api]);
    return (
      <TaskOptimisticContextProvider
        value={{
          task,
          applyPatch: (patch) => setTask((current) => ({ ...current, ...patch })),
          restore: setTask,
        }}
      >
        <LabelsPicker task={task} />
      </TaskOptimisticContextProvider>
    );
  }
  render(
    <StateProvider initialState={{ workspaces: { items: [], activeId: seedTask.workspaceId } }}>
      <Harness />
    </StateProvider>,
  );
  return () => store!.getState().office.tasks.items.find((task) => task.id === seedTask.id)?.labels;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  __resetOfficeTaskContentSyncForTests();
});

function activateRemoval(modality: string) {
  const chip = within(screen.getByTestId("labels-picker-trigger")).getByRole("button", {
    name: removeSlashLabelName,
  });
  if (modality === "touch") {
    fireEvent.pointerDown(chip, { pointerType: "touch", pointerId: 1 });
    fireEvent.pointerUp(chip, { pointerType: "touch", pointerId: 1 });
    fireEvent.click(chip, { pointerType: "touch" });
  } else if (modality === "Enter" || modality === " ") {
    fireEvent.keyDown(chip, { key: modality });
  } else {
    fireEvent.click(chip);
  }
}

// @covers AC-TASKS-LABELS-002.6 (shared state/data boundary, not phone geometry or browser reload)
describe("LabelsPicker exact-name removal", () => {
  it.each(["click", "touch", "Enter", " "])(
    "removes through the real client and optimistic hook using %s",
    async (modality) => {
      let resolveResponse!: (response: Response) => void;
      const fetchSpy = vi.fn().mockImplementation(
        () =>
          new Promise<Response>((resolve) => {
            resolveResponse = resolve;
          }),
      );
      vi.stubGlobal("fetch", fetchSpy);
      const storedLabels = renderPicker();

      act(() => activateRemoval(modality));
      expect(fetchSpy).toHaveBeenCalledTimes(1);
      const [request, init] = fetchSpy.mock.calls[0] as [string, RequestInit];
      const url = new URL(request, "https://labels.test");
      expect(url.pathname).toBe(
        "/api/v1/office/workspaces/label-workspace/tasks/label-task/labels",
      );
      expect(url.searchParams.getAll("name")).toEqual(["area/backend"]);
      expect(init.method).toBe("DELETE");
      expect(screen.queryByRole("button", { name: removeSlashLabelName })).toBeNull();
      expect(storedLabels()).toEqual(seedTask.labels.slice(1));

      await act(async () => resolveResponse(Response.json({ ok: true })));
      expect(screen.getByRole("button", { name: "Remove area%2Fbackend" })).toBeTruthy();
      expect(storedLabels()).toEqual(seedTask.labels.slice(1));
      expect(toast.error).not.toHaveBeenCalled();
    },
  );

  it("restores the chip and canonical task labels when removal is rejected", async () => {
    let resolveResponse!: (response: Response) => void;
    const fetchSpy = vi.fn().mockImplementation(
      () =>
        new Promise<Response>((resolve) => {
          resolveResponse = resolve;
        }),
    );
    vi.stubGlobal("fetch", fetchSpy);
    const storedLabels = renderPicker();

    act(() => activateRemoval("click"));
    expect(storedLabels()).toEqual(seedTask.labels.slice(1));
    expect(screen.queryByRole("button", { name: removeSlashLabelName })).toBeNull();

    await act(async () =>
      resolveResponse(Response.json({ error: "Removal refused" }, { status: 403 })),
    );

    expect(screen.getByRole("button", { name: removeSlashLabelName })).toBeTruthy();
    expect(storedLabels()).toEqual(seedTask.labels);
    expect(toast.error).toHaveBeenCalledWith("Removal refused");
  });
});
