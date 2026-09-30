import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { PendingChange } from "@/lib/api/domains/coordinator-changes-api";
import {
  SettingsSaveProvider,
  useSettingsSaveContributor,
  useSettingsSaveCoordinator,
  type SettingsSaveCoordinator,
} from "@/components/settings/settings-save-provider";

const listPendingChanges = vi.fn();
const applyPendingChange = vi.fn();
const discardPendingChange = vi.fn();
const getCoordinator = vi.fn();
const searchParams = vi.hoisted(() => ({ value: "" }));

vi.mock("@/lib/api/domains/coordinator-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/coordinator-api")>();
  return { ...actual, getCoordinator: (...a: unknown[]) => getCoordinator(...a) };
});
vi.mock("@/lib/api/domains/coordinator-changes-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/coordinator-changes-api")>();
  return {
    ...actual,
    listPendingChanges: (...a: unknown[]) => listPendingChanges(...a),
    applyPendingChange: (...a: unknown[]) => applyPendingChange(...a),
    discardPendingChange: (...a: unknown[]) => discardPendingChange(...a),
  };
});
vi.mock("@/lib/routing/client-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/routing/client-router")>();
  return { ...actual, useSearchParams: () => new URLSearchParams(searchParams.value) };
});

import { ChangesWaiting } from "./changes-waiting";

let coordinator: SettingsSaveCoordinator | null = null;

function change(id: string, title = "Prefer small tasks"): PendingChange {
  return {
    id,
    coordinator_id: "c1",
    proposal_id: `p-${id}`,
    proposal_title: title,
    field: "context",
    base_value: "old line",
    new_value: "new line",
    status: "pending",
    decided_by: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
  };
}

function Harness({
  canManage = true,
  onContextApplied,
  onSave = async () => undefined,
}: {
  canManage?: boolean;
  onContextApplied?: (context: string) => void;
  onSave?: () => Promise<void>;
}) {
  coordinator = useSettingsSaveCoordinator();
  useSettingsSaveContributor({
    id: "page",
    revision: 1,
    isDirty: true,
    save: onSave,
    discard: () => undefined,
  });
  return (
    <ChangesWaiting
      workspaceId="w1"
      coordinatorId="c1"
      canManage={canManage}
      onContextApplied={onContextApplied}
    />
  );
}

function mount(props: Parameters<typeof Harness>[0] = {}) {
  return render(
    <SettingsSaveProvider>
      <Harness {...props} />
    </SettingsSaveProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  coordinator = null;
  searchParams.value = "";
});

const APPLY = "pending-change-apply";
const EMPTY_TEXT = "No changes waiting for you.";

describe("ChangesWaiting list", () => {
  it("shows the empty state", async () => {
    listPendingChanges.mockResolvedValue({ changes: [] });
    mount();
    await waitFor(() => expect(screen.getByText(EMPTY_TEXT)).not.toBeNull());
  });

  it("shows a load error and retries", async () => {
    listPendingChanges.mockRejectedValueOnce(new Error("x"));
    listPendingChanges.mockResolvedValueOnce({ changes: [change("1")] });
    mount();
    await waitFor(() =>
      expect(screen.getByText("Could not load the changes. Try again.")).not.toBeNull(),
    );
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));
    await waitFor(() => expect(screen.getByText("Prefer small tasks")).not.toBeNull());
  });

  it("readers see the diff and no Apply or Discard", async () => {
    listPendingChanges.mockResolvedValue({ changes: [change("1")] });
    mount({ canManage: false });
    await waitFor(() => expect(screen.getByTestId("context-diff")).not.toBeNull());
    expect(screen.queryByTestId(APPLY)).toBeNull();
    expect(screen.queryByTestId("pending-change-discard")).toBeNull();
  });

  it("Apply refetches the coordinator context and then the list", async () => {
    listPendingChanges
      .mockResolvedValueOnce({ changes: [change("1")] })
      .mockResolvedValueOnce({ changes: [] });
    applyPendingChange.mockResolvedValue({});
    getCoordinator.mockResolvedValue({ context: "new line" });
    const onContextApplied = vi.fn();
    mount({ onContextApplied });
    await waitFor(() => expect(screen.getByTestId(APPLY)).not.toBeNull());
    fireEvent.click(screen.getByTestId(APPLY));
    await waitFor(() => expect(onContextApplied).toHaveBeenCalledWith("new line"));
    await waitFor(() => expect(screen.getByText(EMPTY_TEXT)).not.toBeNull());
    expect(applyPendingChange).toHaveBeenCalledWith("w1", "c1", "1");
  });

  it("Discard removes the row after a refetch", async () => {
    listPendingChanges
      .mockResolvedValueOnce({ changes: [change("1")] })
      .mockResolvedValueOnce({ changes: [] });
    discardPendingChange.mockResolvedValue({});
    mount();
    await waitFor(() => expect(screen.getByTestId("pending-change-discard")).not.toBeNull());
    fireEvent.click(screen.getByTestId("pending-change-discard"));
    await waitFor(() => expect(screen.getByText(EMPTY_TEXT)).not.toBeNull());
  });
});

describe("ChangesWaiting failures", () => {
  it("a context_changed 409 shows its sentence, keeps the row and refetches", async () => {
    listPendingChanges.mockResolvedValue({ changes: [change("1")] });
    applyPendingChange.mockRejectedValue(
      new ApiError("conflict", 409, { error: "conflict", reason: "context_changed" }),
    );
    mount();
    await waitFor(() => expect(screen.getByTestId(APPLY)).not.toBeNull());
    fireEvent.click(screen.getByTestId(APPLY));
    await waitFor(() =>
      expect(
        screen.getByText(
          "The context changed since this was proposed. Discard it, or ask the coordinator to propose again.",
        ),
      ).not.toBeNull(),
    );
    expect(listPendingChanges).toHaveBeenCalledTimes(2);
  });

  it("another 409 reads already settled", async () => {
    listPendingChanges.mockResolvedValue({ changes: [change("1")] });
    applyPendingChange.mockRejectedValue(new ApiError("conflict", 409, { error: "conflict" }));
    mount();
    await waitFor(() => expect(screen.getByTestId(APPLY)).not.toBeNull());
    fireEvent.click(screen.getByTestId(APPLY));
    await waitFor(() =>
      expect(screen.getByText("This change was already settled.")).not.toBeNull(),
    );
  });

  it("a 404 refetches silently", async () => {
    listPendingChanges
      .mockResolvedValueOnce({ changes: [change("1")] })
      .mockResolvedValueOnce({ changes: [] });
    applyPendingChange.mockRejectedValue(new ApiError("missing", 404, null));
    mount();
    await waitFor(() => expect(screen.getByTestId(APPLY)).not.toBeNull());
    fireEvent.click(screen.getByTestId(APPLY));
    await waitFor(() => expect(screen.getByText(EMPTY_TEXT)).not.toBeNull());
    expect(screen.queryByTestId("pending-change-message")).toBeNull();
  });

  it("any other failure keeps the row and shows one plain sentence, never transport text", async () => {
    listPendingChanges.mockResolvedValue({ changes: [change("1")] });
    applyPendingChange.mockRejectedValue(new Error("socket hang up"));
    mount();
    await waitFor(() => expect(screen.getByTestId(APPLY)).not.toBeNull());
    fireEvent.click(screen.getByTestId(APPLY));
    await waitFor(() =>
      expect(screen.getByText("That did not work. Nothing changed. Try again.")).not.toBeNull(),
    );
    expect(screen.queryByText(/socket hang up/)).toBeNull();
    expect(screen.getByText("Prefer small tasks")).not.toBeNull();
    expect(listPendingChanges).toHaveBeenCalledTimes(1);
  });
});

describe("ChangesWaiting exclusion and scroll", () => {
  it("disables Apply while the page save is in flight, and the save waits for an Apply", async () => {
    listPendingChanges.mockResolvedValue({ changes: [change("1")] });
    let releaseSave: () => void = () => undefined;
    const onSave = () =>
      new Promise<void>((resolve) => {
        releaseSave = resolve;
      });
    mount({ onSave });
    await waitFor(() => expect(screen.getByTestId(APPLY)).not.toBeNull());
    let saving: Promise<unknown> = Promise.resolve();
    act(() => {
      saving = coordinator!.saveAll();
    });
    await waitFor(() =>
      expect((screen.getByTestId(APPLY) as HTMLButtonElement).disabled).toBe(true),
    );
    await act(async () => {
      releaseSave();
      await saving;
    });
    await waitFor(() =>
      expect((screen.getByTestId(APPLY) as HTMLButtonElement).disabled).toBe(false),
    );

    let releaseApply: (value: unknown) => void = () => undefined;
    applyPendingChange.mockReturnValue(
      new Promise((resolve) => {
        releaseApply = resolve;
      }),
    );
    getCoordinator.mockResolvedValue({ context: "x" });
    fireEvent.click(screen.getByTestId(APPLY));
    await waitFor(() => expect(coordinator!.exclusiveBusy).toBe(true));
    let result: { canLeave: boolean } = { canLeave: true };
    await act(async () => {
      result = await coordinator!.saveAll();
    });
    expect(result.canLeave).toBe(false);
    await act(async () => {
      releaseApply({});
    });
    await waitFor(() => expect(coordinator!.exclusiveBusy).toBe(false));
  });

  it("scrolls into view once loaded when the URL selects the autonomy section", async () => {
    searchParams.value = "section=autonomy";
    const scroll = vi.fn();
    Element.prototype.scrollIntoView = scroll;
    listPendingChanges.mockResolvedValue({ changes: [] });
    mount();
    await waitFor(() => expect(scroll).toHaveBeenCalledTimes(1));
  });
});
