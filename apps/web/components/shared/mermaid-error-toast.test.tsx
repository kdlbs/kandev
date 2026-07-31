import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import {
  resetMermaidErrorToastHistoryForTest,
  showMermaidErrorToast,
  useMermaidErrorToast,
} from "./mermaid-error-toast";
import { MERMAID_ERROR_EVENT } from "./mermaid-utils";

const mockToast = vi.fn();

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast, updateToast: vi.fn(), dismissToast: vi.fn() }),
}));

beforeEach(() => {
  mockToast.mockClear();
  resetMermaidErrorToastHistoryForTest();
});

describe("useMermaidErrorToast", () => {
  it("calls toast with error details when an event fires", () => {
    const { unmount } = renderHook(() => useMermaidErrorToast());

    act(() => {
      document.dispatchEvent(
        new CustomEvent(MERMAID_ERROR_EVENT, { detail: { message: "Parse error at line 1" } }),
      );
    });

    expect(mockToast).toHaveBeenCalledOnce();
    expect(mockToast).toHaveBeenCalledWith({
      title: "Failed to render diagram",
      description: "Parse error at line 1",
      variant: "error",
    });

    unmount();
  });

  it("shows only one toast after a task-plan listener remount, including a chat failure", () => {
    const first = renderHook(() => useMermaidErrorToast());

    act(() => {
      document.dispatchEvent(
        new CustomEvent(MERMAID_ERROR_EVENT, {
          detail: { message: "Plan parser error", taskId: "task-1" },
        }),
      );
    });
    first.unmount();

    const second = renderHook(() => useMermaidErrorToast());
    act(() => {
      document.dispatchEvent(
        new CustomEvent(MERMAID_ERROR_EVENT, {
          detail: { message: "Plan parser error again", taskId: "task-1" },
        }),
      );
      showMermaidErrorToast(mockToast, "task-1", "Chat parser error");
    });

    expect(mockToast).toHaveBeenCalledOnce();
    second.unmount();
  });

  it("allows a separate task and unscoped failures to show their own toasts", () => {
    const { unmount } = renderHook(() => useMermaidErrorToast());

    act(() => {
      document.dispatchEvent(
        new CustomEvent(MERMAID_ERROR_EVENT, {
          detail: { message: "Task one", taskId: "task-1" },
        }),
      );
      document.dispatchEvent(
        new CustomEvent(MERMAID_ERROR_EVENT, {
          detail: { message: "Task two", taskId: "task-2" },
        }),
      );
      document.dispatchEvent(
        new CustomEvent(MERMAID_ERROR_EVENT, { detail: { message: "Global" } }),
      );
      document.dispatchEvent(
        new CustomEvent(MERMAID_ERROR_EVENT, { detail: { message: "Global again" } }),
      );
    });

    expect(mockToast).toHaveBeenCalledTimes(4);
    unmount();
  });

  it("removes listener on unmount", () => {
    const { unmount } = renderHook(() => useMermaidErrorToast());
    unmount();

    act(() => {
      document.dispatchEvent(new CustomEvent(MERMAID_ERROR_EVENT, { detail: { message: "oops" } }));
    });

    expect(mockToast).not.toHaveBeenCalled();
  });
});
