import { StrictMode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LogViewer } from "./log-viewer";

const createMock = vi.fn();
const fetchMock = vi.fn();
const capabilitiesMock = vi.fn();
const sessionsMock = vi.fn();
const downloadURLMock = vi.fn((..._args: unknown[]) => "/download/bundle-1");

vi.mock("@/lib/api/domains/system-api", () => ({
  createDiagnosticBundle: (...args: unknown[]) => createMock(...args),
  fetchDiagnosticBundle: (...args: unknown[]) => fetchMock(...args),
  fetchDiagnosticBundleCapabilities: (...args: unknown[]) => capabilitiesMock(...args),
  fetchDiagnosticACPSessions: (...args: unknown[]) => sessionsMock(...args),
  buildDiagnosticBundleDownloadUrl: (...args: unknown[]) => downloadURLMock(...args),
}));

beforeEach(() => {
  createMock.mockReset();
  fetchMock.mockReset();
  capabilitiesMock.mockReset();
  sessionsMock.mockReset();
  capabilitiesMock.mockResolvedValue({
    sources: ["backend", "frontend", "runtime"],
    acp_debug_enabled: false,
    acp_max_sessions: 10,
  });
  sessionsMock.mockResolvedValue([]);
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
    fireEvent.click(screen.getByTestId("download-diagnostic-bundle"));
    await waitFor(() => expect(createMock).toHaveBeenCalledWith(["backend", "frontend"], []));
    expect(
      screen.getByRole("button", { name: /Collecting frontend logs/ }).hasAttribute("disabled"),
    ).toBe(true);

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("bundle-1"), { timeout: 1_500 });
    expect(await screen.findByText(/partial ZIP is downloading/)).toBeTruthy();
    expect(downloadURLMock).toHaveBeenCalledWith("bundle-1");
  });

  it("shows the ACP customizer from backend capabilities and requires a session", async () => {
    capabilitiesMock.mockResolvedValue({
      sources: ["backend", "frontend", "runtime", "acp"],
      acp_debug_enabled: true,
      acp_max_sessions: 10,
    });
    sessionsMock.mockResolvedValue([
      {
        task_id: "task-1",
        session_id: "session-1",
        agent: "claude-acp",
        model: "sonnet",
        status: "running",
        executor_type: "local_docker",
        acp_availability: "reachable",
      },
    ]);
    render(<LogViewer />);
    await waitFor(() =>
      expect(screen.getByTestId("download-diagnostic-bundle-with-acp")).toBeTruthy(),
    );
    fireEvent.click(screen.getByTestId("download-diagnostic-bundle-with-acp"));
    expect((await screen.findAllByText("ACP debug messages")).length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText(/can contain prompts/)).toBeTruthy();
    const create = screen.getByTestId("create-custom-diagnostic-bundle");
    expect(create.hasAttribute("disabled")).toBe(true);
    fireEvent.click(await screen.findByRole("checkbox", { name: "session-1" }));
    await waitFor(() => expect(create.hasAttribute("disabled")).toBe(false));
  });
});
