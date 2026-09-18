import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import type { ImprovementDetail } from "@/lib/api/domains/assistant-maintenance-types";
import { MaintenanceReview } from "./maintenance-review";
const api = vi.hoisted(() => ({ run: vi.fn(), revoke: vi.fn() }));
vi.mock("@/lib/api/domains/assistant-maintenance-api", () => ({
  runMaintenance: api.run,
  revokeMaintenanceGrant: api.revoke,
}));
vi.mock("./maintenance-results", () => ({
  MaintenanceEvidence: () => null,
  MaintenanceFileView: () => null,
  MaintenanceArtifactView: () => null,
  MaintenanceResolution: () => <span data-testid="recovery-picker" />,
}));
const binding = {
  id: "b",
  owner_user_id: "owner",
  version: 2,
  intent_revision: 4,
  execution_mode: "execute",
} as AssistantBinding;
const detail = {
  candidate: { id: "proposal", state: "investigating", revision: 3, repair_task_id: "review-task" },
  grant: {
    revision: 5,
    binding_version: 2,
    expires_at: "2099-01-01T00:00:00Z",
    scope: { files: ["example.js"] },
  },
  validation: null,
  review: null,
} as ImprovementDetail;
beforeEach(() => {
  api.run.mockReset().mockResolvedValue({});
  api.revoke.mockReset().mockResolvedValue({});
});
afterEach(cleanup);
it("requires current authority and passing checks before local commit", async () => {
  const updated = vi.fn();
  const { rerender } = render(
    <MaintenanceReview binding={binding} detail={detail} onUpdated={updated} />,
  );
  const commit = () =>
    screen.getByRole<HTMLButtonElement>("button", { name: "Create local commit" });
  expect(commit().disabled).toBe(true);
  expect(screen.queryByTestId("recovery-picker")).toBeNull();
  const passed = {
    ...detail,
    validation: { tree_oid: "tree", grant_revision: 5, passed: true, checks: [] },
  };
  rerender(
    <MaintenanceReview
      binding={{ ...binding, paused: true }}
      detail={passed}
      onUpdated={updated}
    />,
  );
  expect(commit().disabled).toBe(true);
  rerender(<MaintenanceReview binding={binding} detail={passed} onUpdated={updated} />);
  await act(async () => {
    fireEvent.click(commit());
  });
  expect(api.run).toHaveBeenCalledWith(
    "proposal",
    expect.objectContaining({
      action: "commit",
      grant_revision: 5,
      candidate_revision: 3,
      expected_binding_version: 2,
    }),
  );
  expect(updated).toHaveBeenCalledOnce();
  rerender(
    <MaintenanceReview
      binding={binding}
      detail={{ ...passed, grant: { ...passed.grant!, revoked_at: "2026-09-18T01:00:00Z" } }}
      onUpdated={updated}
    />,
  );
  expect(commit().disabled).toBe(true);
});
it("revokes the displayed grant with its exact revision without executing maintenance", async () => {
  render(<MaintenanceReview binding={binding} detail={detail} onUpdated={vi.fn()} />);
  await act(async () => {
    fireEvent.click(screen.getByRole("button", { name: "Revoke preparation grant" }));
  });
  expect(api.revoke).toHaveBeenCalledWith("proposal", 2, 5);
  expect(api.run).not.toHaveBeenCalled();
});
