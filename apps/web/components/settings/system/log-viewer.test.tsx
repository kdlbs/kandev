import { StrictMode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LogViewer } from "./log-viewer";

const createMock = vi.fn();
const fetchMock = vi.fn();
const capabilitiesMock = vi.fn();
const sessionsMock = vi.fn();
const downloadURLMock = vi.fn((..._args: unknown[]) => "/download/bundle-1");
const CHECKED_STATE = "checked";
const DATA_STATE_ATTRIBUTE = "data-state";

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
  it("uses one customizer with the standard backend and frontend defaults", async () => {
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
    expect(screen.queryByTestId("download-diagnostic-bundle")).toBeNull();
    expect(screen.queryByTestId("download-diagnostic-bundle-with-acp")).toBeNull();
    fireEvent.click(screen.getByTestId("customize-diagnostic-bundle"));
    expect(
      screen.getByRole("checkbox", { name: "Backend logs" }).getAttribute(DATA_STATE_ATTRIBUTE),
    ).toBe(CHECKED_STATE);
    expect(
      screen.getByRole("checkbox", { name: "Frontend logs" }).getAttribute(DATA_STATE_ATTRIBUTE),
    ).toBe(CHECKED_STATE);
    expect(
      screen.getByRole("checkbox", { name: "Runtime index" }).getAttribute(DATA_STATE_ATTRIBUTE),
    ).not.toBe(CHECKED_STATE);
    fireEvent.click(screen.getByTestId("create-custom-diagnostic-bundle"));
    await waitFor(() => expect(createMock).toHaveBeenCalledWith(["backend", "frontend"], []));

    await waitFor(() => expect(fetchMock).toHaveBeenCalledWith("bundle-1"), { timeout: 1_500 });
    expect(await screen.findByText(/partial ZIP is downloading/)).toBeTruthy();
    expect(downloadURLMock).toHaveBeenCalledWith("bundle-1");
  });

  it("offers ACP in the one customizer with task links and bounded bulk selection", async () => {
    capabilitiesMock.mockResolvedValue({
      sources: ["backend", "frontend", "runtime", "acp"],
      acp_debug_enabled: true,
      acp_max_sessions: 1,
    });
    sessionsMock.mockResolvedValue([
      {
        task_id: "task-1",
        task_title: "Repair failing diagnostics",
        session_id: "session-1",
        agent: "claude-acp",
        model: "sonnet",
        status: "running",
        executor_type: "local_docker",
        acp_availability: "reachable",
      },
      {
        task_id: "task-2",
        task_title: "Inspect timeout",
        session_id: "session-2",
        agent: "codex-acp",
        acp_availability: "host_retained",
      },
    ]);
    render(<LogViewer />);
    await waitFor(() => expect(screen.getByTestId("customize-diagnostic-bundle")).toBeTruthy());
    expect(screen.queryByTestId("download-diagnostic-bundle")).toBeNull();
    expect(screen.queryByTestId("download-diagnostic-bundle-with-acp")).toBeNull();
    fireEvent.click(screen.getByTestId("customize-diagnostic-bundle"));
    fireEvent.click(screen.getByRole("checkbox", { name: "ACP debug messages" }));
    expect((await screen.findAllByText("ACP debug messages")).length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText(/can contain prompts/)).toBeTruthy();
    const create = screen.getByTestId("create-custom-diagnostic-bundle");
    expect(create.hasAttribute("disabled")).toBe(true);
    const taskLink = await screen.findByTestId("acp-session-task-link-session-1");
    expect(taskLink.textContent).toContain("Repair failing diagnostics");
    expect(taskLink.getAttribute("href")).toBe("/t/task-1");
    expect(taskLink.getAttribute("target")).toBe("_blank");
    fireEvent.click(screen.getByTestId("select-all-acp-sessions"));
    await waitFor(() => expect(create.hasAttribute("disabled")).toBe(false));
    expect(screen.getByText("1 session selected")).toBeTruthy();
    expect(
      screen.getByRole("checkbox", { name: "session-1" }).getAttribute(DATA_STATE_ATTRIBUTE),
    ).toBe(CHECKED_STATE);
    expect(
      screen.getByRole("checkbox", { name: "session-2" }).getAttribute(DATA_STATE_ATTRIBUTE),
    ).toBe("unchecked");
    fireEvent.click(screen.getByTestId("clear-acp-session-selection"));
    await waitFor(() => expect(create.hasAttribute("disabled")).toBe(true));
  });
});
