import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { AssistantBinding, AssistantInputSnapshot } from "@/lib/api/domains/assistant-api";
import { AttentionCard } from "./attention-card";
const api = vi.hoisted(() => ({ get: vi.fn(), resolve: vi.fn() }));
vi.mock("@/lib/api/domains/assistant-api", () => ({
  getAssistantInput: api.get,
  resolveAssistantInput: api.resolve,
}));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => ({ updateMessage: vi.fn() }) }),
}));
vi.mock("@/components/routing/app-link", () => ({
  default: ({ href, children }: { href: string; children: React.ReactNode }) => (
    <a href={href}>{children}</a>
  ),
}));
const binding = { id: "b", version: 1, intent_revision: 4 } as AssistantBinding;
const snapshot: AssistantInputSnapshot = {
  attention: {
    id: "a",
    binding_id: "b",
    workspace_id: "ws",
    task_id: "task",
    session_id: "session",
    source_id: "source",
    source_revision: "native-revision",
    revision: 3,
    summary: "Example input",
    updated_at: "2026-09-18T12:00:00Z",
    kind: "permission",
    state: "pending",
  },
  input: {
    kind: "permission",
    state: "pending",
    source_id: "source",
    source_revision: "native-revision",
    task_id: "task",
    session_id: "session",
    pending_id: "pending",
    profile_id: "profile",
    summary: "Example input",
    permission: {
      title: "Read the sample guide",
      task_id: "task",
      session_id: "session",
      pending_id: "pending",
      request_id: "request",
      action: { type: "read", path: "example.md", redacted: false },
      options: [
        { option_id: "provider-allow", name: "Allow this read", kind: "allow_once" },
        { option_id: "provider-deny", name: "Decline", kind: "reject_once" },
      ],
    },
  },
};
beforeEach(() => {
  api.get.mockReset().mockResolvedValue(snapshot);
  api.resolve.mockReset().mockResolvedValue({});
});
afterEach(cleanup);
it("shows only native offered choices and resolves the exact native tuple", async () => {
  render(
    <AttentionCard
      binding={binding}
      row={snapshot.attention}
      onResolved={vi.fn()}
      onDismiss={vi.fn()}
    />,
  );
  const allow = await screen.findByRole("button", { name: "Allow this read" });
  expect(screen.queryByRole("button", { name: "Always allow" })).toBeNull();
  await act(async () => {
    fireEvent.click(allow);
  });
  expect(api.resolve).toHaveBeenCalledWith(
    "a",
    expect.objectContaining({
      option_id: "provider-allow",
      expected_revision: 3,
      source_revision: "native-revision",
      session_id: "session",
      expected_intent_revision: 4,
    }),
  );
  expect(screen.getByRole("link").getAttribute("href")).toBe("/tasks/task?sessionId=session");
});
it("replaces expired native input with its actual status and no action buttons", async () => {
  api.get.mockResolvedValue({ ...snapshot, input: { ...snapshot.input, state: "expired" } });
  render(
    <AttentionCard
      binding={binding}
      row={snapshot.attention}
      onResolved={vi.fn()}
      onDismiss={vi.fn()}
    />,
  );
  await waitFor(() => expect(screen.getByRole("status").textContent).toContain("Expired"));
  expect(screen.queryByRole("button", { name: "Allow this read" })).toBeNull();
  expect(api.resolve).not.toHaveBeenCalled();
});
