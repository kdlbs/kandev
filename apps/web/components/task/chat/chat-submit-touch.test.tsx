import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { SubmitButton } from "./chat-input-toolbar-primitives";

vi.mock("@/components/state-provider", () => ({
  useAppStore: () => false,
  useAppStoreApi: () => ({ getState: () => ({ chatInput: { cancellingBySessionId: {} } }) }),
}));
vi.mock("./composer-disclosure", () => ({ useComposerActivity: () => {} }));
vi.mock("./chat-submit-plugin-decoration", () => ({ ChatSubmitPluginDecoration: () => null }));
afterEach(cleanup);

it("keeps the mobile submit target mounted through readiness changes", () => {
  const onSubmit = vi.fn();
  const button = (disabled: boolean) => (
    <TooltipProvider>
      <SubmitButton
        isAgentBusy={false}
        sessionId={null}
        taskId={null}
        hasContent
        isDisabled={disabled}
        isSending={false}
        planModeEnabled={false}
        onCancel={() => {}}
        onSubmit={onSubmit}
        submitShortcut={SHORTCUTS.SUBMIT}
        presentation="mobile"
      />
    </TooltipProvider>
  );
  const view = render(button(false));
  const target = screen.getByTestId("submit-message-button");
  view.rerender(button(true));
  expect(screen.getByTestId("submit-message-button")).toBe(target);
  expect((target as HTMLButtonElement).disabled).toBe(true);
  view.rerender(button(false));
  expect(screen.getByTestId("submit-message-button")).toBe(target);
  fireEvent.click(target);
  expect(onSubmit).toHaveBeenCalledTimes(1);
});
