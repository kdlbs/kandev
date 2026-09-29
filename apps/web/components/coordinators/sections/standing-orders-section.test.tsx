import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";

const listMock = vi.fn();
const retireMock = vi.fn();
const restoreMock = vi.fn();
const toastSuccess = vi.fn();
const toastError = vi.fn();

vi.mock("@/lib/toast/sonner", () => ({
  toast: {
    success: (...args: unknown[]) => toastSuccess(...args),
    error: (...args: unknown[]) => toastError(...args),
  },
}));
vi.mock("@/hooks/domains/coordinator/use-standing-orders", async () => {
  const React = await import("react");
  return {
    useStandingOrders: () => {
      const [state, setState] = React.useState<{ orders: unknown[]; status: string }>({
        orders: [],
        status: "loading",
      });
      const reload = React.useCallback(() => {
        return Promise.resolve(listMock()).then(
          (orders) => setState({ orders, status: "ready" }),
          () => setState((prev) => (prev.status === "ready" ? prev : { ...prev, status: "error" })),
        );
      }, []);
      React.useEffect(() => {
        void reload();
      }, [reload]);
      return { ...state, reload, retry: reload };
    },
  };
});
vi.mock("@/lib/api/domains/coordinator-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/coordinator-api")>();
  return {
    ...actual,
    retireStandingOrder: (...args: unknown[]) => retireMock(...args),
    restoreStandingOrder: (...args: unknown[]) => restoreMock(...args),
  };
});

import { StandingOrdersSection } from "./standing-orders-section";

const RETIRE = "standing-order-retire";

function order(id: string, number: number, text: string, applied: string | null = null) {
  return {
    id,
    number,
    text,
    created_at: "2026-09-12T10:00:00Z",
    created_by: "u",
    retired_at: null,
    last_applied_at: applied,
  };
}

function renderSection(canManage = true) {
  render(<StandingOrdersSection workspaceId="ws" coordinatorId="c" canManage={canManage} />);
}

beforeEach(() => {
  [listMock, retireMock, restoreMock, toastSuccess, toastError].forEach((m) => m.mockReset());
});
afterEach(cleanup);

describe("StandingOrdersSection", () => {
  it("lists orders with number, text and never-applied, and shows empty text", async () => {
    listMock.mockResolvedValue([order("a", 1, "Prefer small cards.")]);
    renderSection();
    expect(await screen.findByText("Standing order 1")).toBeTruthy();
    expect(screen.getByText("Prefer small cards.")).toBeTruthy();
    expect(document.body.textContent).toContain("Never applied");
  });

  it("shows the empty text when there are no orders", async () => {
    listMock.mockResolvedValue([]);
    renderSection();
    expect(await screen.findByText("There are no standing orders yet.")).toBeTruthy();
  });

  it("gives readers the list without any control or fresh-conversation note", async () => {
    listMock.mockResolvedValue([order("a", 1, "Rule")]);
    renderSection(false);
    await screen.findByText("Rule");
    expect(screen.queryByTestId(RETIRE)).toBeNull();
    expect(screen.queryByTestId("standing-order-add")).toBeNull();
  });

  it("shows the load error state with retry", async () => {
    listMock.mockRejectedValueOnce(new Error("x"));
    renderSection();
    expect(await screen.findByTestId("standing-orders-error")).toBeTruthy();
    listMock.mockResolvedValue([]);
    fireEvent.click(screen.getByText("Retry"));
    expect(await screen.findByTestId("standing-orders-empty")).toBeTruthy();
  });

  it("retires with the row number in the toast, offers a 10 s Undo and restores on Undo", async () => {
    listMock.mockResolvedValueOnce([order("a", 1, "One"), order("b", 2, "Two")]);
    retireMock.mockResolvedValue({ ...order("b", 2, "Two"), number: null });
    restoreMock.mockResolvedValue(order("b", 2, "Two"));
    renderSection();
    await screen.findByText("Two");
    listMock.mockResolvedValue([order("a", 1, "One")]);
    fireEvent.click(screen.getAllByTestId(RETIRE)[1]);
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    const [message, options] = toastSuccess.mock.calls[0];
    expect(message).toBe("Standing order 2 retired.");
    expect(options.duration).toBe(10_000);
    expect(options.action.label).toBe("Undo");
    await waitFor(() => expect(screen.queryByText("Two")).toBeNull());
    listMock.mockResolvedValue([order("a", 1, "One"), order("b", 2, "Two")]);
    await act(async () => options.action.onClick());
    expect(restoreMock).toHaveBeenCalledWith("ws", "c", "b");
    expect(await screen.findByText("Two")).toBeTruthy();
  });

  it("disables Retire while in flight so one click sends one request", async () => {
    listMock.mockResolvedValue([order("a", 1, "One")]);
    let resolve: (v: unknown) => void = () => {};
    retireMock.mockReturnValue(new Promise((r) => (resolve = r)));
    renderSection();
    await screen.findByText("One");
    const button = screen.getByTestId(RETIRE) as HTMLButtonElement;
    fireEvent.click(button);
    expect(button.disabled).toBe(true);
    fireEvent.click(button);
    expect(retireMock).toHaveBeenCalledTimes(1);
    await act(async () => resolve({ ...order("a", 1, "One"), number: null }));
  });
});

describe("StandingOrdersSection retire single-flight", () => {
  it("keeps Retire disabled until the list reloads so a second click sends no request and no second toast", async () => {
    listMock.mockResolvedValueOnce([order("a", 1, "One")]);
    retireMock.mockResolvedValue({ ...order("a", 1, "One"), number: null });
    renderSection();
    await screen.findByText("One");
    let release: (v: unknown[]) => void = () => {};
    listMock.mockReturnValue(new Promise((r) => (release = r)));
    const button = screen.getByTestId(RETIRE) as HTMLButtonElement;
    fireEvent.click(button);
    await waitFor(() => expect(toastSuccess).toHaveBeenCalledTimes(1));
    expect(button.disabled).toBe(true);
    fireEvent.click(button);
    expect(retireMock).toHaveBeenCalledTimes(1);
    await act(async () => release([]));
    await waitFor(() => expect(screen.queryByText("One")).toBeNull());
    expect(toastSuccess).toHaveBeenCalledTimes(1);
  });

  it("shows an error toast without Undo when retire fails and keeps the list", async () => {
    listMock.mockResolvedValue([order("a", 1, "One")]);
    retireMock.mockRejectedValue(new Error("500"));
    renderSection();
    await screen.findByText("One");
    fireEvent.click(screen.getByTestId(RETIRE));
    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith("Could not retire the standing order."),
    );
    expect(toastSuccess).not.toHaveBeenCalled();
    expect(screen.getByText("One")).toBeTruthy();
  });

  it("explains an Undo answered 404 and one refused at the limit, then re-lists", async () => {
    listMock.mockResolvedValue([order("a", 1, "One")]);
    retireMock.mockResolvedValue({ ...order("a", 1, "One"), number: null });
    renderSection();
    await screen.findByText("One");
    fireEvent.click(screen.getByTestId(RETIRE));
    await waitFor(() => expect(toastSuccess).toHaveBeenCalled());
    const { onClick } = toastSuccess.mock.calls[0][1].action;
    restoreMock.mockRejectedValueOnce(new ApiError("nf", 404, {}));
    await act(async () => onClick());
    expect(toastError).toHaveBeenCalledWith("That standing order no longer exists.");
    restoreMock.mockRejectedValueOnce(
      new ApiError("l", 400, { error_code: "standing_order_limit" }),
    );
    await act(async () => onClick());
    expect(toastError).toHaveBeenCalledWith(
      "You can have up to 20 standing orders. Retire one to add another.",
    );
    expect(listMock.mock.calls.length).toBeGreaterThanOrEqual(4);
  });
});
