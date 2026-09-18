import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { AssistantBinding } from "@/lib/api/domains/assistant-api";
import type { ImprovementDetail } from "@/lib/api/domains/assistant-maintenance-types";
import { MaintenanceGrantForm } from "./maintenance-grant-form";
vi.mock("@/hooks/domains/orchestration/use-assistant-maintenance", () => ({
  useMaintenancePage: () => ({
    entries: [],
    nextCursor: "",
    loading: false,
    loaded: true,
    refresh: vi.fn(),
  }),
}));
const binding = { version: 2 } as AssistantBinding;
const detail = {
  candidate: { id: "proposal", revision: 3 },
  grant: {
    revision: 4,
    scope: {
      repository_id: "repo",
      workflow_id: "workflow",
      workflow_step_id: "review",
      profile_id: "profile",
      files: ["scripts/example.js"],
      actions: ["read", "patch", "test", "commit"],
      image: "synthetic-image",
      positive_check: ["node", "positive.js"],
      negative_check: ["node", "negative.js"],
    },
  },
} as ImprovementDetail;
afterEach(cleanup);
it("rejects malformed check arguments before submitting an explicit scoped grant", () => {
  const save = vi.fn();
  render(<MaintenanceGrantForm binding={binding} detail={detail} busy={false} onSave={save} />);
  fireEvent.change(screen.getByTestId("maintenance-negative"), {
    target: { value: "run all commands" },
  });
  fireEvent.submit(screen.getByTestId("maintenance-grant-form"));
  expect(save).not.toHaveBeenCalled();
  expect(screen.getByRole("alert")).toBeTruthy();
  fireEvent.change(screen.getByTestId("maintenance-negative"), {
    target: { value: '["node","negative.js"]' },
  });
  fireEvent.submit(screen.getByTestId("maintenance-grant-form"));
  expect(save).toHaveBeenCalledOnce();
  expect(save.mock.calls[0][0]).toMatchObject({
    expected_binding_version: 2,
    expected_revision: 4,
    candidate_revision: 3,
    scope: {
      files: ["scripts/example.js"],
      actions: ["read", "patch", "test", "commit"],
      negative_check: ["node", "negative.js"],
      repository_id: "repo",
    },
  });
});
