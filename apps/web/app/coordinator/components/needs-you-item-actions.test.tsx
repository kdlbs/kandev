import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { AttentionTask, NeedsYouStallItem } from "@/lib/coordinator/attention";
import { WebSocketRequestTimeoutError } from "@/lib/ws/request-error";

const launchMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: (...args: unknown[]) => launchMock(...args),
}));

import { Phase2CardProvider } from "../proposal-card/phase2-context";
import { resetStallResumesForTest } from "../use-stall-resume";
import { NeedsYouItemPrimaryActions, resumableSessionId } from "./needs-you-item-actions";

beforeEach(() => {
  launchMock.mockReset();
  resetStallResumesForTest();
});
afterEach(cleanup);

function stall(state: string | undefined, hasSession = true): NeedsYouStallItem {
  const task: AttentionTask = {
    id: "t-2",
    title: "T",
    identifier: "KAN-2",
    statusSummary: hasSession
      ? { primary_session: { id: "s-1", ...(state ? { state } : {}) } }
      : { primary_session: null },
  };
  return {
    kind: "stall",
    id: "t-2",
    task,
    stall: { task_id: "t-2", stalled_for_ms: 1, last_event_at: "x", detected_at: "y" },
    referenceTimeMs: 0,
    ageMs: 0,
  } as NeedsYouStallItem;
}

function renderActions(
  item: NeedsYouStallItem,
  opts: { enabled?: boolean; canManage?: boolean } = {},
) {
  return render(
    <Phase2CardProvider
      value={{ enabled: opts.enabled ?? true, orders: undefined, offerReject: undefined }}
    >
      <NeedsYouItemPrimaryActions item={item} canManage={opts.canManage ?? true} />
    </Phase2CardProvider>,
  );
}

describe("resumableSessionId", () => {
  it("needs a primary session that is neither COMPLETED nor CREATED", () => {
    expect(resumableSessionId(stall("FAILED").task)).toBe("s-1");
    expect(resumableSessionId(stall(undefined).task)).toBe("s-1");
    expect(resumableSessionId(stall("COMPLETED").task)).toBeUndefined();
    expect(resumableSessionId(stall("CREATED").task)).toBeUndefined();
    expect(resumableSessionId(stall("RUNNING", false).task)).toBeUndefined();
  });
});

describe("stall Resume button", () => {
  it("is absent for a reader, with the flag off, and for a non-resumable session", () => {
    renderActions(stall("FAILED"), { canManage: false });
    expect(screen.queryByRole("button", { name: "Resume" })).toBeNull();
    cleanup();
    renderActions(stall("FAILED"), { enabled: false });
    expect(screen.queryByRole("button", { name: "Resume" })).toBeNull();
    cleanup();
    renderActions(stall("COMPLETED"));
    expect(screen.queryByRole("button", { name: "Resume" })).toBeNull();
  });

  it("keeps Open task and Show the evidence beside Resume", () => {
    renderActions(stall("FAILED"));
    expect(screen.getByRole("button", { name: "Resume" })).not.toBeNull();
    expect(screen.getByRole("link", { name: "Open task" })).not.toBeNull();
    expect(screen.getByRole("button", { name: "Show the evidence" })).not.toBeNull();
  });

  it("disables while in flight, sends once, then reads Resuming with a stable name and a status", async () => {
    let resolve!: (value: unknown) => void;
    launchMock.mockReturnValue(new Promise((res) => (resolve = res)));
    renderActions(stall("FAILED"));
    const button = screen.getByRole("button", { name: "Resume" }) as HTMLButtonElement;
    fireEvent.click(button);
    fireEvent.click(button);
    expect(launchMock).toHaveBeenCalledTimes(1);
    expect(button.disabled).toBe(true);
    await act(async () => resolve({ success: true }));
    const after = screen.getByRole("button", { name: "Resume" }) as HTMLButtonElement;
    expect(after.textContent).toBe("Resuming");
    expect(after.disabled).toBe(true);
    expect(screen.getByRole("status").textContent).toBe("Resuming this task.");
  });

  it("shows Resume queued for a queued activation", async () => {
    launchMock.mockResolvedValue({ success: true, activation_disposition: "queued" });
    renderActions(stall("FAILED"));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Resume" }));
    });
    expect(screen.getByRole("button", { name: "Resume" }).textContent).toBe("Resume queued");
  });

  it("shows a generic alert on refusal, never the raw text, and re-enables the button", async () => {
    launchMock.mockResolvedValue({ success: false, error: "raw agentctl failure" });
    renderActions(stall("FAILED"));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Resume" }));
    });
    expect(screen.getByRole("alert").textContent).toBe("Could not resume. Try again.");
    expect(screen.queryByText(/raw agentctl/)).toBeNull();
    expect((screen.getByRole("button", { name: "Resume" }) as HTMLButtonElement).disabled).toBe(
      false,
    );
  });

  it("says the outcome is unknown after a timeout", async () => {
    launchMock.mockRejectedValue(new WebSocketRequestTimeoutError("session.launch"));
    renderActions(stall("FAILED"));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Resume" }));
    });
    expect(screen.getByRole("alert").textContent).toContain("unknown whether the task resumed");
  });
});
