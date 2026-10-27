import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { defaultFeaturesState } from "@/lib/state/slices/features/features-slice";
import type { AppState } from "@/lib/state/store";
import type { WorkloadRunObservation } from "@/lib/types/background-work";
import { BackgroundWorkPanel } from "./background-work-panel";
import { BackgroundWorkChip } from "./background-work-chip";

const SESSION = "session-clear";
const WORK = "work-clear";
const CAP = 200 * 1024;
const BUTTON = "button";
const CLEAR = "Clear view";
const FRESH = "fresh-after-clear";
const workload: WorkloadRunObservation = {
  work_id: WORK,
  run_id: "run-1",
  title: "Build watcher",
  kind: "shell",
  state: "running",
  revision: 1,
  capabilities: {
    discovery: "snapshot",
    output: "stream",
    transcript: false,
    parentage: false,
    reasoning_summary: false,
    attributable_usage: false,
  },
};

vi.mock("@/lib/api/domains/background-work-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/background-work-api")>()),
  listBackgroundWorkloads: vi.fn(async () => ({ workloads: [workload] })),
}));

let store: StoreApi<AppState>;
function CaptureStore() {
  store = useAppStoreApi();
  return null;
}

async function mountDetail() {
  const result = render(
    <StateProvider
      initialState={{ features: { ...defaultFeaturesState.features, agentBackgroundWork: true } }}
    >
      <CaptureStore />
      <BackgroundWorkPanel panelId="detail" params={{ sessionId: SESSION, workId: WORK }} />
    </StateProvider>,
  );
  await screen.findByTestId(`background-work-detail-${WORK}`);
  return result;
}

function append(chunk: string) {
  act(() =>
    store.getState().appendBackgroundWorkloadOutput(SESSION, { work_id: WORK, chunk, offset: 0 }),
  );
}

function displayedOutput() {
  return screen.getByTestId(`background-work-detail-${WORK}`).querySelector("pre")?.textContent;
}

beforeEach(() => {
  HTMLElement.prototype.scrollIntoView = vi.fn();
});
afterEach(() => cleanup());

describe("background workload clear with a rolling window", () => {
  it("shows new retained output after Clear view when the window is full", async () => {
    await mountDetail();
    append("old".repeat(Math.ceil(CAP / 3)).slice(0, CAP));
    fireEvent.click(screen.getByRole(BUTTON, { name: CLEAR }));
    append(`\n${FRESH}\n`);
    const retained = store.getState().backgroundWork.workloadsBySessionId[SESSION]![0]!;
    expect(retained.output).toContain(FRESH);
    expect(retained.output).toHaveLength(CAP);
    expect(retained.output_truncated).toBe(true);
    expect(displayedOutput()).toContain(FRESH);
  });

  it("shows the same new output without clearing", async () => {
    await mountDetail();
    append("old".repeat(Math.ceil(CAP / 3)).slice(0, CAP));
    append(`\n${FRESH}\n`);
    await waitFor(() => expect(displayedOutput()).toContain(FRESH));
  });

  it("handles repeated clears, search, rollover past the anchor, and auto-scroll", async () => {
    await mountDetail();
    const scroll = HTMLElement.prototype.scrollIntoView as ReturnType<typeof vi.fn>;
    append("old\n");
    fireEvent.click(screen.getByRole(BUTTON, { name: CLEAR }));
    append("keep first\ndrop this\n");
    expect(displayedOutput()).toBe("keep first\ndrop this\n");
    fireEvent.change(screen.getByPlaceholderText("Search output..."), {
      target: { value: "keep" },
    });
    expect(displayedOutput()).toBe("keep first");
    fireEvent.change(screen.getByPlaceholderText("Search output..."), { target: { value: "" } });
    fireEvent.click(screen.getByRole(BUTTON, { name: CLEAR }));
    fireEvent.click(screen.getByRole(BUTTON, { name: "Auto-scroll: on" }));
    const calls = scroll.mock.calls.length;
    append("é😀" + "z".repeat(CAP) + "\nnew-tail\n");
    expect(displayedOutput()).toContain("new-tail");
    expect(displayedOutput()).not.toContain("keep first");
    expect(scroll.mock.calls).toHaveLength(calls);
    expect(screen.getByText(/Earlier output truncated/i)).toBeTruthy();
    fireEvent.click(screen.getByRole(BUTTON, { name: "Auto-scroll: off" }));
    expect(scroll.mock.calls.length).toBeGreaterThan(calls);
  });

  it("keeps a clear through identical refresh and status, and isolates a replacement run", async () => {
    await mountDetail();
    append("before-clear");
    fireEvent.click(screen.getByRole(BUTTON, { name: CLEAR }));
    const current = store.getState().backgroundWork.workloadsBySessionId[SESSION]![0]!;
    act(() => {
      store.getState().setBackgroundWorkloads(SESSION, [{ ...current }]);
      store
        .getState()
        .updateBackgroundWorkload(SESSION, { ...workload, revision: 2, state: "waiting" });
    });
    expect(displayedOutput()).toBe("Live output unavailable");
    append(" fresh");
    expect(displayedOutput()).toBe(" fresh");
    act(() =>
      store.getState().updateBackgroundWorkload(SESSION, {
        ...workload,
        revision: 3,
        output: "replacement-output",
      }),
    );
    expect(displayedOutput()).toBe("replacement-output");
    act(() =>
      store.getState().updateBackgroundWorkload(SESSION, {
        ...workload,
        revision: 4,
        run_id: "run-2",
        output: "next-run-output",
      }),
    );
    expect(displayedOutput()).toBe("next-run-output");
  });
});

describe("background workload view isolation and phone output", () => {
  it("keeps Clear view local to one detail view", async () => {
    render(
      <StateProvider
        initialState={{ features: { ...defaultFeaturesState.features, agentBackgroundWork: true } }}
      >
        <CaptureStore />
        <BackgroundWorkPanel panelId="first" params={{ sessionId: SESSION, workId: WORK }} />
        <BackgroundWorkPanel panelId="second" params={{ sessionId: SESSION, workId: WORK }} />
      </StateProvider>,
    );
    await waitFor(() =>
      expect(screen.getAllByTestId(`background-work-detail-${WORK}`)).toHaveLength(2),
    );
    append("shared old\n");
    fireEvent.click(screen.getAllByRole(BUTTON, { name: CLEAR })[0]!);
    append("shared new\n");
    const output = screen
      .getAllByTestId(`background-work-detail-${WORK}`)
      .map((detail) => detail.querySelector("pre")?.textContent);
    expect(output).toEqual(["shared new\n", "shared old\nshared new\n"]);
  });

  it("does not carry a clear across selected workloads or sessions", async () => {
    const result = await mountDetail();
    append("first workload");
    fireEvent.click(screen.getByRole(BUTTON, { name: CLEAR }));
    act(() =>
      store
        .getState()
        .setBackgroundWorkloads(SESSION, [
          store.getState().backgroundWork.workloadsBySessionId[SESSION]![0]!,
          { ...workload, work_id: "second", output: "second workload" },
        ]),
    );
    result.rerender(
      <StateProvider
        initialState={{ features: { ...defaultFeaturesState.features, agentBackgroundWork: true } }}
      >
        <CaptureStore />
        <BackgroundWorkPanel panelId="detail" params={{ sessionId: SESSION, workId: "second" }} />
      </StateProvider>,
    );
    expect(screen.getByText("second workload")).toBeTruthy();
    fireEvent.click(screen.getByRole(BUTTON, { name: CLEAR }));
    act(() =>
      store
        .getState()
        .setBackgroundWorkloads("another-session", [
          { ...workload, work_id: "second", output: "other session" },
        ]),
    );
    result.rerender(
      <StateProvider
        initialState={{ features: { ...defaultFeaturesState.features, agentBackgroundWork: true } }}
      >
        <CaptureStore />
        <BackgroundWorkPanel
          panelId="detail"
          params={{ sessionId: "another-session", workId: "second" }}
        />
      </StateProvider>,
    );
    expect(screen.getByText("other session")).toBeTruthy();
  });

  it("renders the existing phone drawer output after opening a workload", async () => {
    const previousWidth = window.innerWidth;
    Object.defineProperty(window, "innerWidth", { configurable: true, value: 390 });
    try {
      render(
        <StateProvider
          initialState={{
            features: { ...defaultFeaturesState.features, agentBackgroundWork: true },
          }}
        >
          <CaptureStore />
          <BackgroundWorkChip sessionId={SESSION} />
        </StateProvider>,
      );
      await screen.findByTestId("background-work-chip");
      append("phone output marker");
      fireEvent.click(screen.getByTestId("background-work-chip"));
      await screen.findByTestId("background-work-drawer-content");
      fireEvent.click(screen.getByTestId(`background-work-open-button-${WORK}`));
      expect(await screen.findByText("phone output marker")).toBeTruthy();
      expect(screen.getByTestId("background-work-drawer-back")).toBeTruthy();
      expect(screen.queryByRole(BUTTON, { name: CLEAR })).toBeNull();
    } finally {
      cleanup();
      Object.defineProperty(window, "innerWidth", { configurable: true, value: previousWidth });
    }
  });
});
