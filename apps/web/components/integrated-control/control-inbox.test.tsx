import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ControlInbox as ControlInboxValue } from "./control-inbox-provider";
import { ControlInbox } from "./control-inbox";

const fixture = vi.hoisted(() => ({ inbox: null as ControlInboxValue | null }));

vi.mock("./integrated-control-provider", () => ({
  useIntegratedControl: () => ({
    data: {
      workspaces: [
        { id: "w1", name: "Alpha" },
        { id: "w2", name: "Beta" },
      ],
    },
  }),
}));
vi.mock("./control-inbox-provider", () => ({ useControlInbox: () => fixture.inbox }));
vi.mock("./control-inbox-rows", () => ({
  ControlAnswerableRow: ({ bundle }: { bundle: { pending_id: string } }) => (
    <div data-testid="answerable-row">{bundle.pending_id}</div>
  ),
  ControlEarlierQuestionRow: ({ bundle }: { bundle: { pending_id: string } }) => (
    <div data-testid="earlier-row">{bundle.pending_id}</div>
  ),
  ControlRecoveryBlockRow: ({ block }: { block: { block_id: string } }) => (
    <div data-testid="recovery-row">{block.block_id}</div>
  ),
}));
vi.mock("@/components/needs-you-inbox/failed-inbox-row", () => ({
  FailedInboxRow: ({ row }: { row: { task_id: string } }) => (
    <div data-testid="failed-row">{row.task_id}</div>
  ),
}));

const ready = <T,>(items: T[]) => ({ status: "ready" as const, items, truncated: false });
const bundle = (id: string) => ({ pending_id: id, created_at: "2026-10-20T01:00:00Z" });

function inbox(over: Partial<ControlInboxValue> = {}): ControlInboxValue {
  return {
    enabled: true,
    loading: false,
    refreshing: false,
    failed: false,
    counts: { pending: 3, answerable: 1, earlier: 1, recovery: 1, failed: 0, partial: true },
    byWorkspace: {},
    reads: [
      {
        workspaceId: "w1",
        answerable: ready([bundle("p1")]),
        earlier: ready([bundle("p2")]),
        recovery: ready([]),
        failed: ready([]),
      },
      {
        workspaceId: "w2",
        answerable: ready([]),
        earlier: ready([]),
        recovery: ready([{ block_id: "b1", created_at: "2026-10-20T01:00:00Z" }]),
        failed: { status: "denied", items: [], truncated: false },
      },
    ] as unknown as ControlInboxValue["reads"],
    ...over,
  };
}

afterEach(cleanup);

describe("ControlInbox", () => {
  it("places each kind in its own section under its workspace", () => {
    fixture.inbox = inbox();
    render(<ControlInbox />);
    const answerable = screen.getByTestId("control-inbox-section-answerable");
    expect(within(answerable).getByTestId("answerable-row").textContent).toBe("p1");
    expect(within(answerable).getByTestId("control-inbox-workspace").dataset.workspaceId).toBe(
      "w1",
    );
    expect(
      within(screen.getByTestId("control-inbox-section-earlier")).getByTestId("earlier-row")
        .textContent,
    ).toBe("p2");
    const recovery = screen.getByTestId("control-inbox-section-recovery");
    expect(within(recovery).getByTestId("control-inbox-workspace").dataset.workspaceId).toBe("w2");
    expect(within(recovery).getByTestId("recovery-row").textContent).toBe("b1");
  });

  it("reports a workspace whose read failed instead of showing it as empty", () => {
    fixture.inbox = inbox();
    render(<ControlInbox />);
    const failed = screen.getByTestId("control-inbox-section-failed");
    expect(within(failed).getByRole("alert")).toBeTruthy();
    expect(within(failed).queryByTestId("failed-row")).toBeNull();
  });

  it("explains a server with the inbox turned off", () => {
    fixture.inbox = inbox({ enabled: false });
    render(<ControlInbox />);
    expect(screen.getByTestId("control-inbox-disabled")).toBeTruthy();
    expect(screen.queryByTestId("control-inbox")).toBeNull();
  });
});
