import { useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { INTEGRATION_STATUS_REFRESH_MS } from "@/hooks/domains/integrations/use-integration-availability";
import { WorkflowSyncSection } from "./workflow-sync-section";
import {
  config,
  Providers,
  transport,
} from "@/hooks/domains/settings/workflow-sync.lifetime.test-helpers";

const { fetchJson } = vi.hoisted(() => ({ fetchJson: vi.fn() }));
vi.mock("@/lib/api/client", async (original) => ({
  ...(await original<typeof import("@/lib/api/client")>()),
  fetchJson,
}));

const SAVE_TEST_ID = "workflow-sync-save";
const DIALOG_TEST_ID = "workflow-sync-dialog";
const URL_TEST_ID = "workflow-sync-url-input";
const REMOVE_TEST_ID = "workflow-sync-remove";
const CONFIRM_TEST_ID = "workflow-sync-remove-confirm";
let wire: ReturnType<typeof transport>;
beforeEach(() => {
  wire = transport();
  fetchJson.mockImplementation(wire.fetch);
  vi.spyOn(window.location, "reload").mockImplementation(() => {});
});
afterEach(async () => {
  cleanup();
  await act(async () => wire.drain());
  vi.restoreAllMocks();
  vi.useRealTimers();
});

function Harness({
  workspace = "A",
  open = true,
  changed,
}: {
  workspace?: string;
  open?: boolean;
  changed?: (open: boolean) => void;
}) {
  const [visible, setVisible] = useState(open);
  return (
    <Providers>
      <button data-testid="reopen" onClick={() => setVisible(true)}>
        reopen
      </button>
      <button data-testid="close" onClick={() => setVisible(false)}>
        close
      </button>
      <WorkflowSyncSection
        workspaceId={workspace}
        dialogOpen={visible}
        onDialogOpenChange={(next) => {
          changed?.(next);
          setVisible(next);
        }}
      />
    </Providers>
  );
}

async function ready() {
  await waitFor(() =>
    expect((screen.getByTestId(SAVE_TEST_ID) as HTMLButtonElement).disabled).toBe(false),
  );
}
function phone(width: number) {
  Object.defineProperty(window, "innerWidth", { configurable: true, value: width });
  window.dispatchEvent(new Event("resize"));
}

// @covers AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.7, AC-INTEGRATIONS-GITLAB-WORKFLOW-SYNC-002.8
describe("real workflow-sync section/dialog transport integration", () => {
  it.each([390, 1024])(
    "a closed/reopened dialog at %s cannot be dismissed by its old save",
    async (width) => {
      phone(width);
      const changed = vi.fn();
      render(<Harness changed={changed} />);
      await ready();
      const saved = wire.hold("POST");
      fireEvent.click(screen.getByTestId(SAVE_TEST_ID));
      expect(wire.requests.filter((r) => r.method === "POST")).toHaveLength(1);
      fireEvent.click(screen.getByTestId("close"));
      fireEvent.click(screen.getByTestId("reopen"));
      await act(async () => saved.resolve(config("A", { branch: "saved" })));
      expect(changed).not.toHaveBeenCalled();
      expect(screen.getByTestId(DIALOG_TEST_ID)).toBeTruthy();
      expect((screen.getByTestId("workflow-sync-branch-input") as HTMLInputElement).value).toBe(
        "saved",
      );
    },
  );

  it("replacement workspace remains open after old save completion", async () => {
    const changed = vi.fn();
    const view = render(<Harness changed={changed} />);
    await ready();
    const saved = wire.hold("POST");
    fireEvent.click(screen.getByTestId(SAVE_TEST_ID));
    view.rerender(<Harness workspace="B" changed={changed} />);
    await ready();
    await act(async () => saved.resolve(config("A")));
    expect(changed).not.toHaveBeenCalled();
    expect((screen.getByTestId(URL_TEST_ID) as HTMLInputElement).value).toContain("/B");
  });

  it("unmounted save cannot publish through the surviving caller", async () => {
    const changed = vi.fn();
    const view = render(<Harness changed={changed} />);
    await ready();
    const saved = wire.hold("POST");
    fireEvent.click(screen.getByTestId(SAVE_TEST_ID));
    view.unmount();
    await act(async () => saved.resolve(config("A")));
    expect(changed).not.toHaveBeenCalled();
  });

  it.each([390, 1024])("old removal at %s cannot dismiss a reopened dialog", async (width) => {
    phone(width);
    const changed = vi.fn();
    render(<Harness changed={changed} />);
    await ready();
    fireEvent.click(screen.getByTestId(REMOVE_TEST_ID));
    const removed = wire.hold("DELETE");
    fireEvent.click(screen.getByTestId(CONFIRM_TEST_ID));
    await waitFor(() => expect(wire.requests.filter((r) => r.method === "DELETE")).toHaveLength(1));
    fireEvent.click(screen.getByTestId("close"));
    fireEvent.click(screen.getByTestId("reopen"));
    await act(async () => removed.resolve({ deleted: true }));
    expect(changed).not.toHaveBeenCalled();
    expect(screen.getByTestId(DIALOG_TEST_ID)).toBeTruthy();
    expect(window.location.reload).toHaveBeenCalledTimes(1);
  });
});

describe("removal target changes", () => {
  it.each([390, 1024])(
    "genuine target read at %s keeps the dialog open after successful removal",
    async (width) => {
      phone(width);
      vi.useFakeTimers();
      const changed = vi.fn();
      await act(async () => render(<Harness changed={changed} />));
      expect((screen.getByTestId(SAVE_TEST_ID) as HTMLButtonElement).disabled).toBe(false);
      const removed = wire.hold("DELETE");
      await act(async () => {
        fireEvent.click(screen.getByTestId(REMOVE_TEST_ID));
      });
      await act(async () => {
        fireEvent.click(screen.getByTestId(CONFIRM_TEST_ID));
      });
      expect(wire.requests.filter((r) => r.method === "DELETE")).toHaveLength(1);
      const read = wire.hold("GET");
      await act(async () => vi.advanceTimersByTime(INTEGRATION_STATUS_REFRESH_MS));
      expect(wire.requests.filter((r) => r.method === "GET")).toHaveLength(2);
      await act(async () => read.resolve(config("A", { repo_name: "replacement" })));
      expect(screen.queryByTestId(CONFIRM_TEST_ID)).toBeNull();
      expect(changed).not.toHaveBeenCalled();
      expect(window.location.reload).not.toHaveBeenCalled();
      await act(async () => removed.resolve({ deleted: true }));
      expect(changed).not.toHaveBeenCalled();
      expect(screen.getByTestId(DIALOG_TEST_ID)).toBeTruthy();
      expect(window.location.reload).toHaveBeenCalledTimes(1);
    },
  );

  it.each([390, 1024])(
    "genuine target edit at %s keeps the dialog open after successful removal",
    async (width) => {
      phone(width);
      const changed = vi.fn();
      render(<Harness changed={changed} />);
      await ready();
      const removed = wire.hold("DELETE");
      fireEvent.click(screen.getByTestId(REMOVE_TEST_ID));
      fireEvent.click(screen.getByTestId(CONFIRM_TEST_ID));
      await waitFor(() =>
        expect(wire.requests.filter((r) => r.method === "DELETE")).toHaveLength(1),
      );
      fireEvent.change(screen.getByTestId(URL_TEST_ID), {
        target: { value: "https://github.com/team/replacement" },
      });
      expect((screen.getByTestId(URL_TEST_ID) as HTMLInputElement).value).toContain("replacement");
      expect(changed).not.toHaveBeenCalled();
      await act(async () => removed.resolve({ deleted: true }));
      expect(changed).not.toHaveBeenCalled();
      expect(screen.getByTestId(DIALOG_TEST_ID)).toBeTruthy();
      expect(window.location.reload).toHaveBeenCalledTimes(1);
      expect(wire.requests.find((r) => r.method === "DELETE")?.workspace).toBe("A");
    },
  );
});

describe("replacement section/dialog settlement", () => {
  it.each([
    { action: "save", reject: true },
    { action: "remove", reject: false },
    { action: "remove", reject: true },
  ])(
    "old $action reject=$reject leaves B's dialog and feedback intact",
    async ({ action, reject }) => {
      const changed = vi.fn();
      const view = render(<Harness changed={changed} />);
      await ready();
      const method = action === "save" ? "POST" : "DELETE";
      const held = wire.hold(method);
      if (action === "save") fireEvent.click(screen.getByTestId(SAVE_TEST_ID));
      else {
        fireEvent.click(screen.getByTestId(REMOVE_TEST_ID));
        fireEvent.click(screen.getByTestId(CONFIRM_TEST_ID));
      }
      await waitFor(() => expect(wire.requests.filter((r) => r.method === method)).toHaveLength(1));
      view.rerender(<Harness workspace="B" changed={changed} />);
      await ready();
      await act(async () => {
        if (reject) held.reject(new Error("old-dialog-failure"));
        else held.resolve({ deleted: true });
      });
      expect(changed).not.toHaveBeenCalled();
      expect(screen.getByTestId(DIALOG_TEST_ID)).toBeTruthy();
      expect((screen.getByTestId(URL_TEST_ID) as HTMLInputElement).value).toContain("/B");
      expect(screen.queryByText(/old-dialog-failure/)).toBeNull();
      expect(window.location.reload).not.toHaveBeenCalled();
    },
  );
});

describe("current section/dialog outcomes", () => {
  it.each(["first", "existing", "provider-switch"])(
    "current %s save dismisses despite its own config/reset",
    async (kind) => {
      if (kind === "first") wire.configs.set("A", undefined);
      const changed = vi.fn();
      render(<Harness changed={changed} />);
      if (kind === "provider-switch") {
        await ready();
        fireEvent.mouseDown(screen.getByRole("tab", { name: "GitLab" }), {
          button: 0,
          ctrlKey: false,
        });
        fireEvent.change(screen.getByTestId(URL_TEST_ID), {
          target: { value: "group/project" },
        });
      } else if (kind === "first") {
        await waitFor(() => expect(wire.requests).toHaveLength(1));
        fireEvent.change(screen.getByTestId(URL_TEST_ID), {
          target: { value: "https://github.com/team/first" },
        });
      }
      await ready();
      fireEvent.click(screen.getByTestId(SAVE_TEST_ID));
      await waitFor(() => expect(changed).toHaveBeenCalledWith(false));
      expect(screen.queryByTestId(DIALOG_TEST_ID)).toBeNull();
    },
  );

  it("save completion uses the committed caller callback", async () => {
    const old = vi.fn();
    const current = vi.fn();
    const view = render(<Harness changed={old} />);
    await ready();
    const saved = wire.hold("POST");
    fireEvent.click(screen.getByTestId(SAVE_TEST_ID));
    view.rerender(<Harness changed={current} />);
    await act(async () => saved.resolve(config()));
    expect(old).not.toHaveBeenCalled();
    expect(current).toHaveBeenCalledWith(false);
  });

  it.each([390, 1024])(
    "current save failure/removal retry at %s retains existing controls",
    async (width) => {
      phone(width);
      const changed = vi.fn();
      render(<Harness changed={changed} />);
      await ready();
      const saved = wire.hold("POST");
      fireEvent.click(screen.getByTestId(SAVE_TEST_ID));
      await act(async () => saved.reject(new Error("current-save-failure")));
      expect(screen.getByText(/current-save-failure/)).toBeTruthy();
      expect(changed).not.toHaveBeenCalled();
      fireEvent.click(screen.getByTestId(REMOVE_TEST_ID));
      const removed = wire.hold("DELETE");
      fireEvent.click(screen.getByTestId(CONFIRM_TEST_ID));
      await act(async () => removed.reject(new Error("current-remove-failure")));
      expect(screen.getByText(/current-remove-failure/)).toBeTruthy();
      fireEvent.click(screen.getByTestId(CONFIRM_TEST_ID));
      await waitFor(() => expect(changed).toHaveBeenCalledWith(false));
      expect(window.location.reload).toHaveBeenCalledTimes(1);
    },
  );
});
