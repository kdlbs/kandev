import { afterEach, describe, expect, it, vi } from "vitest";
import type { RoutineTrigger } from "@/lib/state/slices/office/types";
import { reconcileCronTrigger } from "./cron-reconcile";
import {
  createRoutineTrigger,
  deleteRoutineTrigger,
  listRoutineTriggers,
} from "@/lib/api/domains/office-api";

vi.mock("@/lib/api/domains/office-api", () => ({
  createRoutineTrigger: vi.fn(),
  deleteRoutineTrigger: vi.fn(),
  listRoutineTriggers: vi.fn(),
}));

const createRoutineTriggerMock = vi.mocked(createRoutineTrigger);
const deleteRoutineTriggerMock = vi.mocked(deleteRoutineTrigger);
const listRoutineTriggersMock = vi.mocked(listRoutineTriggers);

afterEach(() => {
  vi.clearAllMocks();
});

const TIMESTAMP = "2026-05-04T00:00:00Z";
const CRON_EXPR = "*/5 * * * *";
const NEW_TRIGGER_ID = "new-trigger";
const DELETE_REJECTED_MESSAGE = "delete rejected";

function makeTrigger(overrides: Partial<RoutineTrigger> = {}): RoutineTrigger {
  return {
    id: "trigger-1",
    routineId: "routine-1",
    kind: "cron",
    cronExpression: CRON_EXPR,
    timezone: "UTC",
    enabled: true,
    createdAt: TIMESTAMP,
    updatedAt: TIMESTAMP,
    ...overrides,
  };
}

describe("reconcileCronTrigger: no-op branches", () => {
  it("does nothing for a webhook draft", async () => {
    const outcome = await reconcileCronTrigger(
      "routine-1",
      { triggerKind: "webhook", cronExpression: "", timezone: "UTC" },
      [],
    );
    expect(outcome).toEqual({ kind: "unchanged" });
    expect(createRoutineTriggerMock).not.toHaveBeenCalled();
  });

  it("does nothing when the cron expression is empty after trimming", async () => {
    const outcome = await reconcileCronTrigger(
      "routine-1",
      { triggerKind: "cron", cronExpression: "   ", timezone: "UTC" },
      [makeTrigger()],
    );
    expect(outcome).toEqual({ kind: "unchanged" });
    expect(createRoutineTriggerMock).not.toHaveBeenCalled();
    expect(deleteRoutineTriggerMock).not.toHaveBeenCalled();
  });

  it("does nothing when the expression and effective timezone are unchanged", async () => {
    const outcome = await reconcileCronTrigger(
      "routine-1",
      { triggerKind: "cron", cronExpression: CRON_EXPR, timezone: "UTC" },
      [makeTrigger()],
    );
    expect(outcome).toEqual({ kind: "unchanged" });
    expect(createRoutineTriggerMock).not.toHaveBeenCalled();
  });

  it("treats an absent stored timezone and an explicit UTC draft as the same effective timezone", async () => {
    const outcome = await reconcileCronTrigger(
      "routine-1",
      { triggerKind: "cron", cronExpression: CRON_EXPR, timezone: "UTC" },
      [makeTrigger({ timezone: undefined })],
    );
    expect(outcome).toEqual({ kind: "unchanged" });
  });

  it("treats a stored UTC timezone and an unset draft timezone as the same effective timezone", async () => {
    const outcome = await reconcileCronTrigger(
      "routine-1",
      { triggerKind: "cron", cronExpression: CRON_EXPR, timezone: "" },
      [makeTrigger({ timezone: "UTC" })],
    );
    expect(outcome).toEqual({ kind: "unchanged" });
    expect(createRoutineTriggerMock).not.toHaveBeenCalled();
  });
});

describe("reconcileCronTrigger: create/delete orchestration", () => {
  it("creates a trigger and skips delete when the routine has no cron trigger yet", async () => {
    createRoutineTriggerMock.mockResolvedValue(makeTrigger({ id: NEW_TRIGGER_ID }));
    listRoutineTriggersMock.mockResolvedValue({ triggers: [makeTrigger({ id: NEW_TRIGGER_ID })] });

    const outcome = await reconcileCronTrigger(
      "routine-1",
      { triggerKind: "cron", cronExpression: CRON_EXPR, timezone: "UTC" },
      [],
    );

    expect(createRoutineTriggerMock).toHaveBeenCalledWith("routine-1", {
      kind: "cron",
      cronExpression: CRON_EXPR,
      timezone: "UTC",
    });
    expect(deleteRoutineTriggerMock).not.toHaveBeenCalled();
    expect(outcome).toEqual({ kind: "success", triggers: [makeTrigger({ id: NEW_TRIGGER_ID })] });
  });

  it("creates the replacement before deleting the existing trigger, then re-lists", async () => {
    const calls: string[] = [];
    createRoutineTriggerMock.mockImplementation(async () => {
      calls.push("create");
      return makeTrigger({ id: NEW_TRIGGER_ID });
    });
    deleteRoutineTriggerMock.mockImplementation(async () => {
      calls.push("delete");
    });
    listRoutineTriggersMock.mockResolvedValue({ triggers: [makeTrigger({ id: NEW_TRIGGER_ID })] });

    const outcome = await reconcileCronTrigger(
      "routine-1",
      { triggerKind: "cron", cronExpression: "0 * * * *", timezone: "UTC" },
      [makeTrigger()],
    );

    expect(calls).toEqual(["create", "delete"]);
    expect(deleteRoutineTriggerMock).toHaveBeenCalledWith("trigger-1");
    expect(outcome).toEqual({ kind: "success", triggers: [makeTrigger({ id: NEW_TRIGGER_ID })] });
  });

  it("leaves the existing trigger in place and issues no delete when create is rejected", async () => {
    createRoutineTriggerMock.mockRejectedValue(new Error("bad cron"));
    listRoutineTriggersMock.mockResolvedValue({ triggers: [makeTrigger()] });

    const outcome = await reconcileCronTrigger(
      "routine-1",
      { triggerKind: "cron", cronExpression: "bad", timezone: "UTC" },
      [makeTrigger()],
    );

    expect(deleteRoutineTriggerMock).not.toHaveBeenCalled();
    expect(outcome).toEqual({
      kind: "create-failed",
      message: "bad cron",
      triggers: [makeTrigger()],
    });
  });

  it("reports create-failed with unknown triggers when the re-list after a create failure also fails", async () => {
    createRoutineTriggerMock.mockRejectedValue(new Error("bad cron"));
    listRoutineTriggersMock.mockRejectedValue(new Error("network down"));

    const outcome = await reconcileCronTrigger(
      "routine-1",
      { triggerKind: "cron", cronExpression: "bad", timezone: "UTC" },
      [makeTrigger()],
    );

    expect(outcome).toEqual({ kind: "create-failed", message: "bad cron", triggers: null });
  });
});

describe("reconcileCronTrigger: delete-failure and stale re-list outcomes", () => {
  it("reports delete-failed with the re-listed triggers when delete fails after a successful create", async () => {
    createRoutineTriggerMock.mockResolvedValue(makeTrigger({ id: NEW_TRIGGER_ID }));
    deleteRoutineTriggerMock.mockRejectedValue(new Error(DELETE_REJECTED_MESSAGE));
    listRoutineTriggersMock.mockResolvedValue({
      triggers: [makeTrigger(), makeTrigger({ id: NEW_TRIGGER_ID })],
    });

    const outcome = await reconcileCronTrigger(
      "routine-1",
      { triggerKind: "cron", cronExpression: "0 * * * *", timezone: "UTC" },
      [makeTrigger()],
    );

    expect(outcome).toEqual({
      kind: "delete-failed",
      message: DELETE_REJECTED_MESSAGE,
      triggers: [makeTrigger(), makeTrigger({ id: NEW_TRIGGER_ID })],
    });
  });

  it("reports delete-failed with unknown triggers when the re-list after a delete failure also fails", async () => {
    createRoutineTriggerMock.mockResolvedValue(makeTrigger({ id: NEW_TRIGGER_ID }));
    deleteRoutineTriggerMock.mockRejectedValue(new Error(DELETE_REJECTED_MESSAGE));
    listRoutineTriggersMock.mockRejectedValue(new Error("network down"));

    const outcome = await reconcileCronTrigger(
      "routine-1",
      { triggerKind: "cron", cronExpression: "0 * * * *", timezone: "UTC" },
      [makeTrigger()],
    );

    expect(outcome).toEqual({
      kind: "delete-failed",
      message: DELETE_REJECTED_MESSAGE,
      triggers: null,
    });
  });

  it("reports success with null triggers (stale) when the re-list fails after create and delete both succeed", async () => {
    createRoutineTriggerMock.mockResolvedValue(makeTrigger({ id: NEW_TRIGGER_ID }));
    deleteRoutineTriggerMock.mockResolvedValue(undefined);
    listRoutineTriggersMock.mockRejectedValue(new Error("network down"));

    const outcome = await reconcileCronTrigger(
      "routine-1",
      { triggerKind: "cron", cronExpression: "0 * * * *", timezone: "UTC" },
      [makeTrigger()],
    );

    expect(outcome).toEqual({ kind: "success", triggers: null });
  });
});
