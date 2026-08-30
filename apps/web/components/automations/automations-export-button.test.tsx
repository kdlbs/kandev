import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mockExportAutomationsZip = vi.fn();
vi.mock("@/lib/api/domains/automations-export-api", () => ({
  exportAutomationsZip: (...args: unknown[]) => mockExportAutomationsZip(...args),
}));

const mockTriggerBlobDownload = vi.fn();
vi.mock("@/lib/utils/file-download", () => ({
  triggerBlobDownload: (...args: unknown[]) => mockTriggerBlobDownload(...args),
}));

const mockToastError = vi.fn();
vi.mock("@/lib/toast/sonner", () => ({
  toast: {
    error: (...args: unknown[]) => mockToastError(...args),
    success: vi.fn(),
    info: vi.fn(),
  },
}));

// window.open / anchor href navigation are the pinned anti-pattern (AC-37); a
// call to either here would mean the control regressed to Office's mechanism.
const windowOpenSpy = vi.spyOn(window, "open").mockImplementation(() => null);

import { AutomationsExportButton } from "./automations-export-button";

const BUTTON_TEST_ID = "export-automations-button";

function getButton(): HTMLButtonElement {
  return screen.getByTestId(BUTTON_TEST_ID) as HTMLButtonElement;
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  mockExportAutomationsZip.mockReset();
  mockTriggerBlobDownload.mockReset();
  mockToastError.mockReset();
  windowOpenSpy.mockClear();
});

afterEach(() => {
  cleanup();
});

describe("AutomationsExportButton", () => {
  it("is enabled up front, including for a workspace with no automations (AC-32/AC-37)", () => {
    render(<AutomationsExportButton workspaceId="ws-empty" />);

    expect(getButton().disabled).toBe(false);
  });

  it("fetches the zip and triggers a download named kandev-automations.zip on success", async () => {
    const blob = new Blob(["zip-bytes"], { type: "application/zip" });
    mockExportAutomationsZip.mockResolvedValue(blob);

    render(<AutomationsExportButton workspaceId="ws-1" />);
    fireEvent.click(getButton());

    await waitFor(() =>
      expect(mockTriggerBlobDownload).toHaveBeenCalledWith(blob, "kandev-automations.zip"),
    );
    expect(mockExportAutomationsZip).toHaveBeenCalledWith("ws-1");
    expect(windowOpenSpy).not.toHaveBeenCalled();
  });

  it("disables the control from the moment the request is issued until it resolves", async () => {
    const pending = deferred<Blob>();
    mockExportAutomationsZip.mockReturnValue(pending.promise);

    render(<AutomationsExportButton workspaceId="ws-1" />);
    const button = getButton();
    fireEvent.click(button);

    await waitFor(() => expect(button.disabled).toBe(true));

    pending.resolve(new Blob(["zip-bytes"]));
    await waitFor(() => expect(button.disabled).toBe(false));
  });

  it("surfaces a non-blocking error and re-enables the control on a non-200 response", async () => {
    mockExportAutomationsZip.mockRejectedValue(new Error("workspace not found"));

    render(<AutomationsExportButton workspaceId="ws-1" />);
    const button = getButton();
    fireEvent.click(button);

    await waitFor(() => expect(mockToastError).toHaveBeenCalledWith("workspace not found"));
    expect(button.disabled).toBe(false);
    expect(mockTriggerBlobDownload).not.toHaveBeenCalled();
  });
});
