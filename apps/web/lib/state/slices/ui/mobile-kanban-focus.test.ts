import { describe, expect, it } from "vitest";
import { create, type StoreApi, type UseBoundStore } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createUISlice } from "./ui-slice";
import type { UISlice } from "./types";

function makeStore(): UseBoundStore<StoreApi<UISlice>> {
  return create<UISlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createUISlice as any)(...a) })),
  );
}
describe("mobileKanban.focusedWorkflowId — phone board focus shared with the drawer", () => {
  it("defaults to null so no workflow is assumed before the board reports one", () => {
    const store = makeStore();

    expect(store.getState().mobileKanban.focusedWorkflowId).toBeNull();
  });

  it("records the workflow the phone board focused, so the drawer configures that one", () => {
    const store = makeStore();

    store.getState().setMobileKanbanFocusedWorkflow("wf-a");

    expect(store.getState().mobileKanban.focusedWorkflowId).toBe("wf-a");
  });

  it("clears back to null when the board leaves phone focus", () => {
    const store = makeStore();
    store.getState().setMobileKanbanFocusedWorkflow("wf-a");

    store.getState().setMobileKanbanFocusedWorkflow(null);

    expect(store.getState().mobileKanban.focusedWorkflowId).toBeNull();
  });
});
