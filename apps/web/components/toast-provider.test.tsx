import { act, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const report = vi.hoisted(() => vi.fn());

vi.mock("@/lib/api/domains/frontend-error-log-api", () => ({
  scheduleFrontendErrorReport: report,
}));

import { ToastProvider, useToast } from "./toast-provider";

describe("ToastProvider error reporting", () => {
  beforeEach(() => {
    report.mockReset();
  });

  it("reports error creation without changing the rendered toast", () => {
    const api = renderProvider();
    let id = "";
    act(() => {
      id = api.toast({
        title: "Failed to save",
        description: "The backend rejected the update",
        variant: "error",
      });
    });

    expect(id).not.toBe("");
    expect(screen.getByText("Failed to save")).toBeTruthy();
    expect(screen.getByText("The backend rejected the update")).toBeTruthy();
    expect(report).toHaveBeenCalledOnce();
    expect(report).toHaveBeenCalledWith({
      source: "toast-provider",
      title: "Failed to save",
      description: "The backend rejected the update",
    });
  });

  it("reports each transition into error exactly once", () => {
    const api = renderProvider();
    let id = "";
    act(() => {
      id = api.toast({ title: "Saving", variant: "loading" });
    });
    expect(report).not.toHaveBeenCalled();

    act(() => api.updateToast(id, { title: "First failure", variant: "error" }));
    act(() => api.updateToast(id, { description: "More detail", variant: "error" }));
    expect(report).toHaveBeenCalledTimes(1);
    expect(report).toHaveBeenLastCalledWith({
      source: "toast-provider",
      title: "First failure",
      description: undefined,
    });

    act(() => api.updateToast(id, { title: "Recovered", variant: "success" }));
    act(() => api.updateToast(id, { title: "Failed again", variant: "error" }));
    expect(report).toHaveBeenCalledTimes(2);
    expect(report).toHaveBeenLastCalledWith({
      source: "toast-provider",
      title: "Failed again",
      description: "More detail",
    });
  });
});

function renderProvider() {
  let current!: ReturnType<typeof useToast>;
  function Capture() {
    current = useToast();
    return null;
  }
  render(
    <ToastProvider>
      <Capture />
    </ToastProvider>,
  );
  return current;
}
