import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";

vi.mock("./subscription-toggle", () => ({ SubscriptionToggle: () => null }));
vi.mock("@/components/state-provider", () => ({ useAppStore: () => undefined }));

import { MRActionButtons, mergeGate } from "./mr-detail-panel";

const openMR = { state: "open", has_conflicts: false, draft: false, detailed_merge_status: "" };
const MERGE_LABEL = "Merge";
const MARK_READY_TESTID = "mr-mark-ready-button";
const IDENTITY = { workspaceId: "ws", project: "g/p", iid: 7, host: "https://gitlab.com" };

afterEach(() => cleanup());

describe("mergeGate", () => {
  it("blocks Merge for a draft and reports it as a draft", () => {
    expect(mergeGate({ ...openMR, draft: true }, false, false)).toEqual({
      draft: true,
      holding: false,
      blocked: true,
    });
    expect(
      mergeGate({ ...openMR, detailed_merge_status: "draft_status" }, false, false).draft,
    ).toBe(true);
  });

  it("lets a draft beat a mergeable status", () => {
    const gate = mergeGate(
      { ...openMR, draft: true, detailed_merge_status: "mergeable" },
      false,
      false,
    );
    expect(gate.draft).toBe(true);
    expect(gate.blocked).toBe(true);
  });

  it("does not call a closed MR a draft but still blocks it", () => {
    const gate = mergeGate({ ...openMR, state: "closed", draft: true }, false, false);
    expect(gate).toEqual({ draft: false, holding: false, blocked: true });
  });

  it("does not require mergeable and does not hold a checking MR outside the wait", () => {
    const gate = mergeGate({ ...openMR, detailed_merge_status: "checking" }, false, false);
    expect(gate.blocked).toBe(false);
  });

  it.each([
    ["conflicts", { ...openMR, has_conflicts: true }, false, false],
    ["busy", openMR, true, false],
    ["holding", openMR, false, true],
  ])("blocks on %s", (_name, mr, busy, holding) => {
    expect(mergeGate(mr, busy, holding).blocked).toBe(true);
  });

  it("is not blocked for an open ready MR", () => {
    expect(mergeGate(openMR, false, false).blocked).toBe(false);
  });
});

function renderButtons(
  mr: Parameters<typeof mergeGate>[0],
  opts: { busy?: boolean; holding?: boolean } = {},
) {
  const onMarkReady = vi.fn();
  const onMerge = vi.fn();
  render(
    <MRActionButtons
      identity={IDENTITY}
      state={mr.state}
      gate={mergeGate(mr, opts.busy ?? false, opts.holding ?? false)}
      busy={opts.busy ?? false}
      run={vi.fn()}
      onMerge={onMerge}
      onMarkReady={onMarkReady}
    />,
  );
  return { onMarkReady, onMerge };
}

describe("MRActionButtons draft state", () => {
  it("shows visible hint text, disabled Merge and an enabled Mark as ready for a draft", () => {
    const { onMarkReady, onMerge } = renderButtons({ ...openMR, draft: true });
    expect(screen.getByTestId("mr-merge-draft-hint").textContent).toContain("draft");
    const merge = screen.getByRole("button", { name: MERGE_LABEL }) as HTMLButtonElement;
    expect(merge.disabled).toBe(true);
    expect(merge.getAttribute("aria-describedby")).toBe(
      screen.getByTestId("mr-merge-draft-hint").id,
    );
    fireEvent.click(screen.getByTestId(MARK_READY_TESTID));
    expect(onMarkReady).toHaveBeenCalledTimes(1);
    expect(onMerge).not.toHaveBeenCalled();
  });

  it("disables Mark as ready while another action is pending", () => {
    renderButtons({ ...openMR, draft: true }, { busy: true });
    expect((screen.getByTestId(MARK_READY_TESTID) as HTMLButtonElement).disabled).toBe(true);
  });

  it("shows no Mark as ready for a non-draft or closed MR", () => {
    renderButtons(openMR);
    expect(screen.queryByTestId(MARK_READY_TESTID)).toBeNull();
    cleanup();
    renderButtons({ ...openMR, state: "closed", draft: true });
    expect(screen.queryByTestId(MARK_READY_TESTID)).toBeNull();
    expect(screen.queryByTestId("mr-merge-draft-hint")).toBeNull();
  });

  it("shows Checking merge status and a disabled Merge during the post-ready hold", () => {
    renderButtons({ ...openMR, detailed_merge_status: "checking" }, { holding: true });
    expect(screen.getByTestId("mr-merge-checking-hint").textContent).toBe("Checking merge status");
    expect((screen.getByRole("button", { name: MERGE_LABEL }) as HTMLButtonElement).disabled).toBe(
      true,
    );
  });

  it("leaves Merge enabled for a checking MR that is not in a wait", () => {
    renderButtons({ ...openMR, detailed_merge_status: "checking" });
    expect((screen.getByRole("button", { name: MERGE_LABEL }) as HTMLButtonElement).disabled).toBe(
      false,
    );
    expect(screen.queryByTestId("mr-merge-checking-hint")).toBeNull();
  });
});
