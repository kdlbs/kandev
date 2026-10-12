import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const addMock = vi.hoisted(() => vi.fn());
const toastMock = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/coordinator-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/coordinator-api")>();
  return { ...actual, addStandingOrder: (...args: unknown[]) => addMock(...args) };
});
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: toastMock }) }));

import { MAX_OFFER_CODE_POINTS, offerPrefill, RejectOfferHost } from "./reject-offer-host";

beforeEach(() => {
  addMock.mockReset().mockResolvedValue({ id: "o-1" });
  toastMock.mockReset();
});
afterEach(cleanup);

const BILLING_RULE = "never touch billing";

describe("offerPrefill", () => {
  it("trims and cuts to the limit in code points", () => {
    expect(offerPrefill("  hi  ")).toBe("hi");
    const cut = Array.from(offerPrefill("😀".repeat(MAX_OFFER_CODE_POINTS + 20)));
    expect(cut).toHaveLength(MAX_OFFER_CODE_POINTS);
  });
});

describe("RejectOfferHost", () => {
  const base = { workspaceId: "ws", coordinatorId: "co" };

  it("renders nothing without an offer", () => {
    render(<RejectOfferHost {...base} offer={null} onClose={vi.fn()} onAdded={vi.fn()} />);
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("prefills the reason, adds the order with its source proposal and toasts", async () => {
    const onClose = vi.fn();
    const onAdded = vi.fn();
    render(
      <RejectOfferHost
        {...base}
        offer={{ proposalId: "p-1", reason: "  never touch billing  " }}
        onClose={onClose}
        onAdded={onAdded}
      />,
    );
    expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe(BILLING_RULE);
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /add|save/i }));
    });
    expect(addMock).toHaveBeenCalledWith("ws", "co", {
      text: BILLING_RULE,
      source_proposal_id: "p-1",
    });
    expect(toastMock).toHaveBeenCalledWith(
      expect.objectContaining({ title: "Standing order added.", variant: "success" }),
    );
    expect(onAdded).toHaveBeenCalled();
  });

  it("keeps the dialog open with an inline error when the save fails", async () => {
    addMock.mockRejectedValueOnce(new Error("boom"));
    const onClose = vi.fn();
    const onAdded = vi.fn();
    render(
      <RejectOfferHost
        {...base}
        offer={{ proposalId: "p-1", reason: BILLING_RULE }}
        onClose={onClose}
        onAdded={onAdded}
      />,
    );
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: /add|save/i }));
    });
    expect(screen.getByRole("alert")).not.toBeNull();
    expect(screen.getByRole("dialog")).not.toBeNull();
    expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe(BILLING_RULE);
    expect(onClose).not.toHaveBeenCalled();
    expect(onAdded).not.toHaveBeenCalled();
    expect(toastMock).not.toHaveBeenCalled();
  });

  it("stays open past the 10 second offer window and closes through onClose", () => {
    vi.useFakeTimers();
    try {
      const onClose = vi.fn();
      render(
        <RejectOfferHost
          {...base}
          offer={{ proposalId: "p-1", reason: "x" }}
          onClose={onClose}
          onAdded={vi.fn()}
        />,
      );
      act(() => {
        vi.advanceTimersByTime(30_000);
      });
      expect(screen.getByRole("dialog")).not.toBeNull();
      fireEvent.keyDown(screen.getByRole("dialog"), { key: "Escape" });
      expect(onClose).toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  it("re-keys on a later offer so its reason wins", () => {
    const { rerender } = render(
      <RejectOfferHost
        {...base}
        offer={{ proposalId: "p-1", reason: "first" }}
        onClose={vi.fn()}
        onAdded={vi.fn()}
      />,
    );
    rerender(
      <RejectOfferHost
        {...base}
        offer={{ proposalId: "p-2", reason: "second" }}
        onClose={vi.fn()}
        onAdded={vi.fn()}
      />,
    );
    expect((screen.getByRole("textbox") as HTMLTextAreaElement).value).toBe("second");
  });
});
