import { describe, expect, it } from "vitest";
import { mapOfficeTaskToTask } from "./page";
import type { OfficeTask } from "@/lib/state/slices/office/types";

const baseRaw: OfficeTask = {
  id: "task-1",
  workspaceId: "workspace-1",
  identifier: "E2E-1",
  title: "Mapper regression task",
  status: "todo",
  priority: "medium",
  createdAt: "2026-05-01T10:00:00Z",
  updatedAt: "2026-05-01T10:00:00Z",
};

describe("mapOfficeTaskToTask", () => {
  // Regression: the detail page seeds its Task from the store-held OfficeTask
  // before the API GET resolves (page.tsx's useIssueData). Dropping rawStatus
  // here silently breaks ExecutionIndicator's Live/Ready sub-state distinction
  // for however long the seeded task is shown — see task-advanced-mode.test.tsx
  // and OfficeSimplePane.test.tsx for the consumer-level regression.
  it("carries rawStatus through when the store holds one", () => {
    const mapped = mapOfficeTaskToTask({ ...baseRaw, status: "todo", rawStatus: "SCHEDULING" });

    expect(mapped.rawStatus).toBe("SCHEDULING");
  });

  it("falls back to the canonical status when rawStatus is absent", () => {
    const mapped = mapOfficeTaskToTask({ ...baseRaw, status: "in_progress" });

    expect(mapped.rawStatus).toBe("in_progress");
  });
});
