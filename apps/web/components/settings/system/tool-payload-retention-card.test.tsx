import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { SettingsSaveContributor } from "@/components/settings/settings-save-provider";
import type { ToolPayloadRetentionStatus } from "@/lib/types/tool-payload-retention";
import * as api from "@/lib/api/domains/tool-payload-retention-api";
import { ToolPayloadRetentionCard } from "./tool-payload-retention-card";
vi.mock("@/lib/api/domains/tool-payload-retention-api");
let contributor: SettingsSaveContributor;
let admin = true;
vi.mock("@/hooks/domains/auth/use-is-admin", () => ({ useIsAdmin: () => admin }));
vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveContributor: (value: SettingsSaveContributor) => {
    contributor = value;
  },
}));
const ENABLED_TEST_ID = "tool-payload-enabled";
const AGE_TEST_ID = "tool-payload-age";

const baseline: ToolPayloadRetentionStatus = {
  supported: true,
  policy: { enabled: false, age: { value: 3, unit: "months" }, revision: 1 },
  preparation: { state: "none", choice: "" },
};
beforeEach(() => {
  vi.resetAllMocks();
  admin = true;
  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValue(baseline);
});
afterEach(cleanup);
async function open() {
  render(<ToolPayloadRetentionCard />);
  await screen.findByTestId(ENABLED_TEST_ID);
}
it("requires an explicit backup choice before saving enablement and displays preparing", async () => {
  await open();
  fireEvent.click(screen.getByTestId(ENABLED_TEST_ID));
  expect(contributor.isDirty).toBe(true);
  expect(contributor.canSave).toBe(false);
  fireEvent.click(screen.getByTestId("tool-payload-backup"));
  expect(contributor.canSave).toBe(true);
  vi.mocked(api.saveToolPayloadRetention).mockResolvedValue({
    ...baseline,
    preparation: { state: "pending", choice: "backup" },
    policy: { ...baseline.policy, revision: 2 },
  });
  await act(async () => {
    await contributor.save(contributor.revision);
  });
  expect(api.saveToolPayloadRetention).toHaveBeenCalledWith({
    ...baseline.policy,
    enabled: true,
    backup_choice: "backup",
  });
  expect(screen.getByTestId(ENABLED_TEST_ID).getAttribute("aria-checked")).toBe("false");
  expect(screen.getByTestId("tool-payload-preparation").textContent).toMatch(/preparing/i);
});
it("analyzes a disabled draft without saving or enabling", async () => {
  await open();
  fireEvent.change(screen.getByTestId(AGE_TEST_ID), { target: { value: "4" } });
  vi.mocked(api.analyzeToolPayloadRetention).mockResolvedValue({ operation_id: "analysis-1" });
  fireEvent.click(screen.getByTestId("tool-payload-analyze"));
  await waitFor(() =>
    expect(api.analyzeToolPayloadRetention).toHaveBeenCalledWith({ value: 4, unit: "months" }),
  );
  expect(api.saveToolPayloadRetention).not.toHaveBeenCalled();
  expect(contributor.isDirty).toBe(true);
});
it("keeps member controls read only", async () => {
  admin = false;
  await open();
  expect(screen.getByTestId(ENABLED_TEST_ID).hasAttribute("disabled")).toBe(true);
  expect(screen.getByTestId("tool-payload-analyze").hasAttribute("disabled")).toBe(true);
  expect(api.analyzeToolPayloadRetention).not.toHaveBeenCalled();
});
it("rejects invalid ages without losing the disabled default", async () => {
  await open();
  fireEvent.change(screen.getByTestId(AGE_TEST_ID), { target: { value: "0" } });
  expect(contributor.canSave).toBe(false);
  expect(screen.getByTestId("tool-payload-analyze").hasAttribute("disabled")).toBe(true);
});

it("preserves edits made during a pending save against its normalized response", async () => {
  await open();
  let resolve!: (value: ToolPayloadRetentionStatus) => void;
  vi.mocked(api.saveToolPayloadRetention).mockReturnValue(
    new Promise((r) => {
      resolve = r;
    }),
  );
  fireEvent.change(screen.getByTestId(AGE_TEST_ID), { target: { value: "4" } });
  let saving!: Promise<void>;
  act(() => {
    saving = Promise.resolve(contributor.save(contributor.revision));
  });
  // A programmatic input can still race a save callback even when controls disable after paint.
  fireEvent.change(screen.getByTestId(AGE_TEST_ID), { target: { value: "5" } });
  await act(async () => {
    resolve({
      ...baseline,
      policy: { ...baseline.policy, revision: 2, age: { value: 4, unit: "months" } },
    });
    await saving;
  });
  expect((screen.getByTestId(AGE_TEST_ID) as HTMLInputElement).value).toBe("5");
  expect(contributor.isDirty).toBe(true);
});
it("shows backup failure after reload and retries the explicitly saved choice", async () => {
  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValue({
    ...baseline,
    preparation: { state: "failed", choice: "backup", error: "backup_failed" },
  });
  await open();
  expect(screen.getByTestId("tool-payload-preparation").textContent).toMatch(/Preparation failed/);
  vi.mocked(api.saveToolPayloadRetention).mockResolvedValue({
    ...baseline,
    preparation: { state: "running", choice: "backup" },
  });
  fireEvent.click(screen.getByTestId("tool-payload-retry"));
  await waitFor(() =>
    expect(api.saveToolPayloadRetention).toHaveBeenCalledWith({
      ...baseline.policy,
      enabled: true,
      backup_choice: "backup",
    }),
  );
});
it("can cancel preparation while policy enablement remains false", async () => {
  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValue({
    ...baseline,
    preparation: { state: "pending", choice: "backup" },
  });
  await open();
  vi.mocked(api.saveToolPayloadRetention).mockResolvedValue(baseline);
  fireEvent.click(screen.getByTestId("tool-payload-cancel-preparation"));
  await waitFor(() => expect(api.saveToolPayloadRetention).toHaveBeenCalledWith(baseline.policy));
});
it("shows unsupported engine capability and prevents mutations", async () => {
  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValue({ ...baseline, supported: false });
  await open();
  expect(screen.getByText(/not supported by this database engine/)).toBeTruthy();
  expect(screen.getByTestId(ENABLED_TEST_ID).hasAttribute("disabled")).toBe(true);
});

it("activates the switch through its visible touch label", async () => {
  await open();
  fireEvent.click(screen.getByText("Enable automatic cleanup"));
  expect(screen.getByTestId(ENABLED_TEST_ID).getAttribute("aria-checked")).toBe("true");
  expect(contributor.canSave).toBe(false);
});
it("keeps an unsaved draft through conflict refresh and sends its original revision", async () => {
  await open();
  vi.useFakeTimers();
  try {
    const current = {
      ...baseline,
      policy: { ...baseline.policy, revision: 2, age: { value: 8, unit: "months" as const } },
    };
    vi.mocked(api.fetchToolPayloadRetention).mockResolvedValue(current);
    fireEvent.change(screen.getByTestId(AGE_TEST_ID), { target: { value: "5" } });
    // Poll timer was installed before fake timers; use the visible refresh path after a save conflict.
    const { ApiError } = await import("@/lib/api/client");
    vi.mocked(api.saveToolPayloadRetention).mockRejectedValue(
      new ApiError("conflict", 409, { code: "conflict" }),
    );
    await act(async () => {
      await expect(contributor.save(contributor.revision)).rejects.toMatchObject({ status: 409 });
    });
    fireEvent.click(screen.getByRole("button", { name: "Refresh status" }));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect((screen.getByTestId(AGE_TEST_ID) as HTMLInputElement).value).toBe("5");
    expect(api.saveToolPayloadRetention).toHaveBeenCalledWith({
      ...baseline.policy,
      age: { value: 5, unit: "months" },
    });
    await act(async () => {
      await contributor.discard?.();
    });
    expect((screen.getByTestId(AGE_TEST_ID) as HTMLInputElement).value).toBe("8");
    expect(contributor.isDirty).toBe(false);
  } finally {
    vi.useRealTimers();
  }
});
it("labels partial analysis as incomplete and shows protected skip reasons", async () => {
  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValue({
    ...baseline,
    last_analysis: {
      id: "partial-scan",
      kind: "analysis",
      state: "partial",
      scanned: 10,
      eligible_tasks: 2,
      eligible_messages: 3,
      removed_messages: 0,
      payload_bytes: 4096,
      skipped: { protected_tasks: 4 },
      cutoff: "2025-09-14T00:00:00Z",
      started_at: "2026-09-14T00:00:00Z",
      finished_at: "2026-09-14T00:01:00Z",
      age: { value: 12, unit: "months" },
    },
  });
  await open();
  expect(screen.getByText(/This result is incomplete/)).toBeTruthy();
  expect(screen.getByText("Protected tasks")).toBeTruthy();
  expect(screen.getByText(/This estimate is stale/)).toBeTruthy();
  expect(screen.queryByText(/No removable payloads found/)).toBeNull();
});

it("can disable an enabled policy even after entering an invalid age", async () => {
  vi.mocked(api.fetchToolPayloadRetention).mockResolvedValue({
    ...baseline,
    policy: { ...baseline.policy, enabled: true },
  });
  await open();
  fireEvent.change(screen.getByTestId(AGE_TEST_ID), { target: { value: "0" } });
  expect(contributor.canSave).toBe(false);
  fireEvent.click(screen.getByTestId(ENABLED_TEST_ID));
  expect(contributor.canSave).toBe(true);
  expect((screen.getByTestId(AGE_TEST_ID) as HTMLInputElement).value).toBe("3");
});
