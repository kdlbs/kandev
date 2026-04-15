import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useMermaidErrorToast } from "./mermaid-error-toast";
import { MERMAID_ERROR_EVENT } from "./mermaid-utils";

const mockToast = vi.fn();

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast, updateToast: vi.fn(), dismissToast: vi.fn() }),
}));

beforeEach(() => {
  mockToast.mockClear();
});

describe("useMermaidErrorToast", () => {
  it("calls toast with error details when event fires", () => {
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

  it("removes listener on unmount", () => {
    const { unmount } = renderHook(() => useMermaidErrorToast());
    unmount();

    act(() => {
      document.dispatchEvent(new CustomEvent(MERMAID_ERROR_EVENT, { detail: { message: "oops" } }));
    });

    expect(mockToast).not.toHaveBeenCalled();
  });
});
