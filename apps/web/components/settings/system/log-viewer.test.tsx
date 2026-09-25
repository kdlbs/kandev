import { StrictMode } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { TauriEventInternals } from "@/lib/desktop/tauri-event-transport";
import { LogViewer } from "./log-viewer";

const createMock = vi.fn();
const fetchMock = vi.fn();
const capabilitiesMock = vi.fn();
const sessionsMock = vi.fn();
const DOWNLOAD_URL = "/download/bundle-1";
const LISTEN_COMMAND_SUFFIX = "|listen";
const DIAGNOSTIC_SAVE_PENDING = "Choose where to save the diagnostic ZIP.";
const DIAGNOSTIC_SAVE_FAILED = "The diagnostic ZIP could not be saved. Try again.";
const CUSTOMIZE_BUNDLE_TEST_ID = "customize-diagnostic-bundle";
const CREATE_BUNDLE_TEST_ID = "create-custom-diagnostic-bundle";
const BUNDLE_STATUS_TEST_ID = "diagnostic-bundle-status";
const downloadURLMock = vi.fn((..._args: unknown[]) => DOWNLOAD_URL);
const CHECKED_STATE = "checked";
const DATA_STATE_ATTRIBUTE = "data-state";
let originalTauriInternals: PropertyDescriptor | undefined;

vi.mock("@/lib/api/domains/system-api", () => ({
  createDiagnosticBundle: (...args: unknown[]) => createMock(...args),
  fetchDiagnosticBundle: (...args: unknown[]) => fetchMock(...args),
  fetchDiagnosticBundleCapabilities: (...args: unknown[]) => capabilitiesMock(...args),
  fetchDiagnosticACPSessions: (...args: unknown[]) => sessionsMock(...args),
  buildDiagnosticBundleDownloadUrl: (...args: unknown[]) => downloadURLMock(...args),
}));

beforeEach(() => {
  originalTauriInternals = Object.getOwnPropertyDescriptor(window, "__TAURI_INTERNALS__");
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
  if (originalTauriInternals) {
    Object.defineProperty(window, "__TAURI_INTERNALS__", originalTauriInternals);
  } else {
    Reflect.deleteProperty(window, "__TAURI_INTERNALS__");
  }
  vi.restoreAllMocks();
});

function installDesktopEvents({ failListener = false } = {}) {
  let onNativeEvent: ((event: { payload: unknown }) => void) | undefined;
  const internals: TauriEventInternals = {
    invoke: vi.fn(async (command: string) => {
      if (failListener && command.endsWith(LISTEN_COMMAND_SUFFIX))
        throw new Error("listener failed");
      return command.endsWith(LISTEN_COMMAND_SUFFIX) ? 41 : undefined;
    }),
    transformCallback: vi.fn((callback) => {
      onNativeEvent = callback;
      return 7;
    }),
  };
  Object.defineProperty(window, "__TAURI_INTERNALS__", { configurable: true, value: internals });
  return {
    internals,
    emit: (payload: unknown) => onNativeEvent?.({ payload }),
  };
}

async function mountDesktopLogViewer(options?: { failListener?: boolean }) {
  createMock.mockResolvedValue({ id: "bundle-1", status: "ready", warnings: [] });
  const desktop = installDesktopEvents(options);
  render(<LogViewer />);
  await waitFor(() =>
    expect(desktop.internals.invoke).toHaveBeenCalledWith(
      "plugin:event|listen",
      expect.objectContaining({ event: "kandev-desktop-v1-download" }),
    ),
  );
  return desktop;
}

function clickDesktopDownload() {
  fireEvent.click(screen.getByTestId(CUSTOMIZE_BUNDLE_TEST_ID));
  fireEvent.click(screen.getByTestId(CREATE_BUNDLE_TEST_ID));
}

async function startDesktopDownload() {
  clickDesktopDownload();
  await screen.findByText(DIAGNOSTIC_SAVE_PENDING);
}

async function prepareDesktopDownload() {
  const desktop = await mountDesktopLogViewer();
  await startDesktopDownload();
  return desktop;
}

describe("LogViewer download feedback", () => {
  it("reports a desktop bundle as saved only after native completion", async () => {
    const desktop = await prepareDesktopDownload();
    expect(screen.queryByText("The diagnostic ZIP was saved.")).toBeNull();
    expect(screen.getByTestId(CUSTOMIZE_BUNDLE_TEST_ID).hasAttribute("disabled")).toBe(true);

    desktop.emit({
      status: "saved",
      url: new URL(DOWNLOAD_URL, window.location.href).href,
      fileName: "kandev-diagnostic-logs.zip",
    });

    expect(await screen.findByText("The diagnostic ZIP was saved.")).toBeTruthy();
  });

  it("keeps native cancellation neutral and the bundle action retryable", async () => {
    const desktop = await prepareDesktopDownload();

    desktop.emit({
      status: "cancelled",
      url: new URL(DOWNLOAD_URL, window.location.href).href,
      fileName: "kandev-diagnostic-logs.zip",
    });

    await waitFor(() => expect(screen.queryByTestId(BUNDLE_STATUS_TEST_ID)).toBeNull());
    expect(screen.getByTestId(CUSTOMIZE_BUNDLE_TEST_ID).hasAttribute("disabled")).toBe(false);
  });

  it("shows a native save error and keeps the bundle action retryable", async () => {
    const desktop = await prepareDesktopDownload();

    desktop.emit({
      status: "failed",
      url: new URL(DOWNLOAD_URL, window.location.href).href,
      fileName: "kandev-diagnostic-logs.zip",
    });

    const status = await screen.findByTestId(BUNDLE_STATUS_TEST_ID);
    expect(status.textContent).toContain(DIAGNOSTIC_SAVE_FAILED);
    expect(screen.getByTestId(CUSTOMIZE_BUNDLE_TEST_ID).hasAttribute("disabled")).toBe(false);
  });

  it("recovers with a retryable error when no native result arrives", async () => {
    await mountDesktopLogViewer();
    const browserSetTimeout = window.setTimeout.bind(window);
    vi.spyOn(window, "setTimeout").mockImplementation(
      (handler, timeout, ...args) =>
        browserSetTimeout(
          handler,
          (timeout ?? 0) >= 600_000 ? 0 : (timeout ?? 0),
          ...args,
        ) as unknown as ReturnType<typeof setTimeout>,
    );

    await startDesktopDownload();

    const status = await screen.findByTestId(BUNDLE_STATUS_TEST_ID);
    expect(status.textContent).toContain(DIAGNOSTIC_SAVE_FAILED);
    expect(screen.getByTestId(CUSTOMIZE_BUNDLE_TEST_ID).hasAttribute("disabled")).toBe(false);
  });

  it("reports a retryable error if the native result listener fails", async () => {
    await mountDesktopLogViewer({ failListener: true });

    clickDesktopDownload();

    const status = await screen.findByTestId(BUNDLE_STATUS_TEST_ID);
    expect(status.textContent).toContain(DIAGNOSTIC_SAVE_FAILED);
    expect(screen.getByTestId(CUSTOMIZE_BUNDLE_TEST_ID).hasAttribute("disabled")).toBe(false);
  });
});

describe("LogViewer customizer", () => {
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
    fireEvent.click(screen.getByTestId(CUSTOMIZE_BUNDLE_TEST_ID));
    expect(
      screen.getByRole("checkbox", { name: "Backend logs" }).getAttribute(DATA_STATE_ATTRIBUTE),
    ).toBe(CHECKED_STATE);
    expect(
      screen.getByRole("checkbox", { name: "Frontend logs" }).getAttribute(DATA_STATE_ATTRIBUTE),
    ).toBe(CHECKED_STATE);
    expect(
      screen.getByRole("checkbox", { name: "Runtime index" }).getAttribute(DATA_STATE_ATTRIBUTE),
    ).not.toBe(CHECKED_STATE);
    fireEvent.click(screen.getByTestId(CREATE_BUNDLE_TEST_ID));
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
    await waitFor(() => expect(screen.getByTestId(CUSTOMIZE_BUNDLE_TEST_ID)).toBeTruthy());
    expect(screen.queryByTestId("download-diagnostic-bundle")).toBeNull();
    expect(screen.queryByTestId("download-diagnostic-bundle-with-acp")).toBeNull();
    fireEvent.click(screen.getByTestId(CUSTOMIZE_BUNDLE_TEST_ID));
    fireEvent.click(screen.getByRole("checkbox", { name: "ACP debug messages" }));
    expect((await screen.findAllByText("ACP debug messages")).length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText(/can contain prompts/)).toBeTruthy();
    const create = screen.getByTestId(CREATE_BUNDLE_TEST_ID);
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
