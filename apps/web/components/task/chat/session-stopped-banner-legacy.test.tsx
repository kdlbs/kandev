import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { SessionStoppedBanner } from "./session-stopped-banner";

const mocks = vi.hoisted(() => ({
  request: vi.fn(),
  recoveryBlocks: [] as Record<string, unknown>[],
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) =>
    selector({
      agentProfiles: { items: [{ id: "profile-1" }] },
      taskSessions: {
        items: {
          "session-1": {
            agent_profile_id: "profile-1",
            session_recovery_blocks: mocks.recoveryBlocks,
          },
        },
      },
    }),
}));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({ request: mocks.request }) }));
vi.mock("@/hooks/domains/session/use-session-actions", () => ({
  useSessionActions: () => ({ stop: vi.fn() }),
}));
vi.mock("@/components/task/new-session-dialog", () => ({ NewSessionDialog: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

const TASK_ID = "task-1";
const SESSION_ID = "session-1";
const SESSION_RECOVER_ACTION = "session.recover";
const RESUME_BUTTON_TEST_ID = "recovery-resume-button";
const RETRY_CONNECTION_BUTTON = "recovery-retry-connection-button";
const STOP_BUTTON = "recovery-stop-button";
afterEach(() => {
  cleanup();
  mocks.recoveryBlocks.length = 0;
  mocks.request.mockReset();
});

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.8
it.each([
  { reason: "unresolved_durable_work", delivery_submission_id: "" },
  { reason: "unknown_prompt_outcome", delivery_submission_id: "cancelled-prompt" },
])("offers explicit Resume for $reason without replaying a prompt", async (block) => {
  mocks.recoveryBlocks.push({
    id: "legacy-fence",
    consumer_reference: "agent_delivery",
    ...block,
    delivery_stream_id: "",
  });
  let complete: (value: unknown) => void = () => {};
  mocks.request.mockImplementationOnce(
    () =>
      new Promise((resolve) => {
        complete = resolve;
      }),
  );
  render(
    <TooltipProvider>
      <SessionStoppedBanner
        taskId={TASK_ID}
        sessionId={SESSION_ID}
        mode="recoverable"
        uncertainDelivery
        showDialog={false}
        onShowDialog={() => {}}
      />
    </TooltipProvider>,
  );
  expect(mocks.request).not.toHaveBeenCalled();
  expect(screen.getByText("task:deliveryRecoveryLegacyResume")).toBeTruthy();
  expect(screen.queryByText("task:deliveryRecoveryMissingSubmission")).toBeNull();
  expect(screen.getByTestId(RETRY_CONNECTION_BUTTON)).toBeTruthy();
  expect(screen.getByTestId(STOP_BUTTON)).toBeTruthy();
  fireEvent.click(screen.getByTestId(RESUME_BUTTON_TEST_ID));
  expect(mocks.request).toHaveBeenCalledOnce();
  expect(mocks.request).toHaveBeenCalledWith(
    SESSION_RECOVER_ACTION,
    { task_id: TASK_ID, session_id: SESSION_ID, action: "resume" },
    expect.anything(),
  );
  expect(screen.getByTestId(RESUME_BUTTON_TEST_ID)).toHaveProperty("disabled", true);
  complete({ success: true });
  await waitFor(() =>
    expect(screen.getByTestId(RESUME_BUTTON_TEST_ID)).toHaveProperty("disabled", false),
  );
});
