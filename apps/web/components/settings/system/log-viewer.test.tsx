import { StrictMode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LogViewer } from "./log-viewer";

const createMock = vi.fn();
const fetchMock = vi.fn();
const downloadURLMock = vi.fn((..._args: unknown[]) => "/download/bundle-1");

vi.mock("@/lib/api/domains/system-api", () => ({
  createDiagnosticBundle: (...args: unknown[]) => createMock(...args),
  fetchDiagnosticBundle: (...args: unknown[]) => fetchMock(...args),
  buildDiagnosticBundleDownloadUrl: (...args: unknown[]) => downloadURLMock(...args),
}));

beforeEach(() => {
  createMock.mockReset();
  fetchMock.mockReset();
  downloadURLMock.mockClear();
  vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

describe("LogViewer", () => {
  it("shows one disclosure-first combined workflow and downloads partial bundles", async () => {
    createMock.mockResolvedValue({
      id: "bundle-1",
      status: "collecting",
      warnings: [],
    });
    fetchMock.mockResolvedValue({
      id: "bundle-1",
      status: "partial",
      warnings: ["One browser did not respond."],
    });
    render(
      <StrictMode>
        <LogViewer />
      </StrictMode>,
    );

    expect(screen.getByText("Review before sharing")).toBeTruthy();
    expect(screen.queryByText("Recent log output")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Download diagnostic bundle" }));
    await waitFor(() => expect(createMock).toHaveBeenCalledWith(["backend", "frontend"]));
    expect(
      screen.getByRole("button", { name: /Collecting frontend logs/ }).hasAttribute("disabled"),
    ).toBe(true);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("bundle-1"), { timeout: 1_500 });
    expect(await screen.findByText(/partial ZIP is downloading/)).toBeTruthy();
    expect(downloadURLMock).toHaveBeenCalledWith("bundle-1");
  });
});
