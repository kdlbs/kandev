import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { IntegrationChangeRequestStatus } from "./integration-change-request-status";
import type { IntegrationChangeRequestStatusItem } from "./integration-change-request-status-types";

const mocks = vi.hoisted(() => ({ touch: false }));
const STATUS_POPOVER_TEST_ID = "integration-change-request-status-popover";

vi.mock("@/hooks/use-compact-task-chrome", () => ({
  useTouchDrawer: () => mocks.touch,
}));

beforeEach(() => {
  mocks.touch = false;
});

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

function statusItem(overrides: Partial<IntegrationChangeRequestStatusItem> = {}) {
  const onRefresh = vi.fn();
  const onOpenReview = vi.fn();
  const onUnlink = vi.fn(async () => undefined);
  return {
    item: {
      id: "change-42",
      number: 42,
      title: "Provider-neutral change",
      repositoryLabel: "acme/demo",
      url: "https://example.com/pull/42",
      state: "open",
      status: "pending",
      pipelineRows: [
        {
          id: "build",
          label: "Build",
          state: "success",
          detail: "Passed in 1m",
          url: "https://ci.example.com/build/42",
        },
        {
          id: "test",
          label: "Tests",
          state: "pending",
          detail: "Running",
          url: "https://ci.example.com/test/42",
        },
      ],
      onRefresh,
      onOpenReview,
      onUnlink,
      ...overrides,
    } satisfies IntegrationChangeRequestStatusItem,
    onRefresh,
    onOpenReview,
    onUnlink,
  };
}

function renderStatus(overrides: Partial<IntegrationChangeRequestStatusItem> = {}) {
  const first = statusItem(overrides);
  render(<IntegrationChangeRequestStatus items={[first.item]} />);
  return first;
}

function renderComposerStatus(overrides: Partial<IntegrationChangeRequestStatusItem> = {}) {
  const first = statusItem(overrides);
  render(<IntegrationChangeRequestStatus items={[first.item]} surface="composer" />);
  return first;
}

describe("IntegrationChangeRequestStatus", () => {
  it("opens the native review on click and reveals pipeline status after 150ms hover", async () => {
    vi.useFakeTimers();
    const { onOpenReview, onRefresh } = renderStatus();
    const trigger = screen.getByRole("button", { name: /#42 Provider-neutral change/ });

    expect(trigger.className).toContain("outline");
    fireEvent.click(trigger);
    expect(onOpenReview).toHaveBeenCalledOnce();

    fireEvent.mouseEnter(trigger);
    act(() => vi.advanceTimersByTime(149));
    expect(screen.queryByTestId(STATUS_POPOVER_TEST_ID)).toBeNull();
    act(() => vi.advanceTimersByTime(1));
    const popover = screen.getByTestId(STATUS_POPOVER_TEST_ID);
    expect(popover).toBeTruthy();
    expect(screen.getByTestId("pr-topbar-popover-inner").parentElement).toBe(popover);
    const passed = popover.querySelector("[data-testid='pr-check-group'][data-kind='passed']");
    expect(passed?.querySelector("[data-testid='pr-check-group-count']")?.textContent).toBe("1");
    expect(popover.querySelector("[data-testid='pr-workflow-open']")).toBeTruthy();
    expect(screen.getByText("Tests")).toBeTruthy();
    expect(onRefresh).toHaveBeenCalledOnce();
  });

  it("matches the GitHub single topbar icon DOM and sizing", () => {
    renderStatus({ status: "failure" });

    const trigger = screen.getByTestId("integration-change-request-status-trigger");
    expect(trigger.children).toHaveLength(3);
    expect(trigger.children[0]?.classList.contains("h-4")).toBe(true);
    expect(trigger.children[1]?.textContent).toBe("#42");
    expect(trigger.children[2]?.classList.contains("h-3")).toBe(true);
  });

  it("matches the GitHub composer hover wrapper and leaves click as a no-op", () => {
    vi.useFakeTimers();
    const { onOpenReview } = renderComposerStatus();
    const chip = screen.getByTestId("integration-change-request-status-chip");

    expect(chip.parentElement?.tagName).toBe("SPAN");
    expect(chip.parentElement?.className).toContain("inline-flex");
    fireEvent.mouseEnter(chip.parentElement!);
    act(() => vi.advanceTimersByTime(150));
    expect(screen.getByTestId(STATUS_POPOVER_TEST_ID)).toBeTruthy();
    fireEvent.click(chip);
    expect(onOpenReview).not.toHaveBeenCalled();
    expect(screen.getByTestId(STATUS_POPOVER_TEST_ID)).toBeTruthy();
  });

  it("supports keyboard focus and keeps the popover open across its portal hover bridge", () => {
    vi.useFakeTimers();
    renderStatus();
    const trigger = screen.getByRole("button", { name: /#42 Provider-neutral change/ });

    fireEvent.focus(trigger);
    act(() => vi.advanceTimersByTime(150));
    const popover = screen.getByTestId(STATUS_POPOVER_TEST_ID);

    fireEvent.blur(trigger);
    fireEvent.mouseEnter(popover);
    act(() => vi.advanceTimersByTime(300));
    expect(screen.getByTestId(STATUS_POPOVER_TEST_ID)).toBeTruthy();

    fireEvent.mouseLeave(popover);
    act(() => vi.advanceTimersByTime(150));
    expect(screen.queryByTestId(STATUS_POPOVER_TEST_ID)).toBeNull();
  });

  it("exposes a shared unlink control in the desktop status popover", async () => {
    vi.useFakeTimers();
    const { onUnlink } = renderStatus();
    const trigger = screen.getByRole("button", { name: /#42 Provider-neutral change/ });
    fireEvent.mouseEnter(trigger);
    act(() => vi.advanceTimersByTime(150));

    fireEvent.click(screen.getByRole("button", { name: "Unlink pull request #42" }));
    await act(async () => Promise.resolve());

    expect(onUnlink).toHaveBeenCalledOnce();
  });

  it("aborts a provider unlink when its host-owned popover unmounts", async () => {
    vi.useFakeTimers();
    let unlinkSignal: AbortSignal | undefined;
    const onUnlink = vi.fn((signal?: AbortSignal) => {
      unlinkSignal = signal;
      return new Promise<void>(() => {});
    });
    const item = statusItem({ onUnlink });
    const rendered = render(<IntegrationChangeRequestStatus items={[item.item]} />);
    const trigger = screen.getByRole("button", { name: /#42 Provider-neutral change/ });
    fireEvent.mouseEnter(trigger);
    act(() => vi.advanceTimersByTime(150));

    fireEvent.click(screen.getByRole("button", { name: "Unlink pull request #42" }));
    await act(async () => Promise.resolve());
    expect(unlinkSignal).toBeInstanceOf(AbortSignal);

    rendered.unmount();
    expect(unlinkSignal?.aborted).toBe(true);
  });
});

describe("IntegrationChangeRequestStatus touch and multi-review parity", () => {
  it("keeps the touch topbar lifecycle aligned with GitHub and opens review directly", () => {
    mocks.touch = true;
    const { onOpenReview } = renderStatus();
    const trigger = screen.getByRole("button", { name: /#42 Provider-neutral change/ });
    fireEvent.click(trigger);

    expect(onOpenReview).toHaveBeenCalledOnce();
    expect(screen.queryByTestId("integration-change-request-status-drawer")).toBeNull();
  });

  it("uses the exact composer chip and bounded drawer anatomy on touch", async () => {
    mocks.touch = true;
    const { onOpenReview, onRefresh } = renderComposerStatus();
    const trigger = screen.getByRole("button", { name: /#42 Provider-neutral change/ });
    expect(trigger.className).toContain("inline-flex");
    expect(trigger.className).not.toContain("h-11");
    fireEvent.click(trigger);

    const drawer = await screen.findByTestId("integration-change-request-status-drawer");
    expect(onRefresh).toHaveBeenCalledOnce();
    expect(drawer.className).toContain("max-h-[80dvh]");
    expect(
      screen.getByTestId("integration-change-request-status-drawer-header").className.split(/\s+/),
    ).toEqual(
      expect.arrayContaining([
        "flex",
        "flex-row",
        "items-center",
        "justify-between",
        "border-b",
        "py-2",
      ]),
    );
    expect(screen.getByTestId("integration-change-request-status-drawer-close")).toBeTruthy();
    const body = screen.getByTestId("integration-change-request-status-scroll-body");
    expect(body.className).toContain("overflow-y-auto");
    expect(body.className).toContain("min-h-0");
    expect(body.querySelectorAll(".overflow-y-auto")).toHaveLength(0);

    const review = screen.getByRole("button", { name: "Open review" });
    expect(review.className).toContain("h-11");
    fireEvent.click(review);
    expect(onRefresh).toHaveBeenCalledOnce();
    expect(onOpenReview).toHaveBeenCalledOnce();
  });

  it("keeps unlink touch-sized inside the mobile status drawer", async () => {
    mocks.touch = true;
    const { onUnlink } = renderComposerStatus();
    fireEvent.click(screen.getByRole("button", { name: /#42 Provider-neutral change/ }));
    await screen.findByTestId("integration-change-request-status-drawer");

    const unlink = screen.getByRole("button", { name: "Unlink pull request #42" });
    expect(unlink.className).toContain("h-11");
    fireEvent.click(unlink);
    await act(async () => Promise.resolve());

    expect(onUnlink).toHaveBeenCalledOnce();
  });

  it("renders one aggregate control and dropdown for multiple linked changes", async () => {
    vi.useFakeTimers();
    const first = statusItem();
    const second = statusItem({
      id: "change-43",
      number: 43,
      title: "Second provider-neutral change",
      status: "failure",
      pipelineRows: [{ id: "lint", label: "Lint", state: "failure" }],
    });
    render(<IntegrationChangeRequestStatus items={[first.item, second.item]} />);

    const trigger = screen.getByRole("button", { name: "2 pull requests" });
    expect(screen.getAllByTestId("integration-change-request-status-trigger")).toHaveLength(1);
    fireEvent.mouseEnter(trigger);
    act(() => vi.advanceTimersByTime(150));
    expect(screen.getByTestId("integration-change-request-status-popover")).toBeTruthy();
    expect(screen.getByText("Lint")).toBeTruthy();
    expect(first.onRefresh).toHaveBeenCalledOnce();
    expect(second.onRefresh).toHaveBeenCalledOnce();
    fireEvent.mouseLeave(trigger);
    act(() => vi.advanceTimersByTime(150));
    fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false });
    const firstItem = screen.getByRole("menuitem", { name: /acme\/demo #42/ });
    expect(firstItem.children).toHaveLength(3);
    expect(firstItem.children[0]?.className).toContain("shrink-0");
    expect(firstItem.children[1]?.className).toContain("flex-col");
    expect(firstItem.children[2]?.classList.contains("h-3")).toBe(true);
    expect(screen.getByRole("menuitem", { name: /acme\/demo #43/ })).toBeTruthy();
  });
});
