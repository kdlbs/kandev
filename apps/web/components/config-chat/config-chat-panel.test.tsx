import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { ConfigChatHeader } from "./config-chat-header";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false }),
}));

const SESSION = {
  sessionId: "session-1",
  taskId: "task-1",
  workspaceId: "workspace-1",
  kind: "config" as const,
};
const RESTART_LABEL = "configChat:restartSession";

afterEach(cleanup);

function headerProps() {
  return { session: SESSION, busy: false, onRestart: vi.fn(), onExpand: vi.fn(), onClose: vi.fn() };
}

describe("configuration chat header confirmation", () => {
  it("cancels without dispatching and confirms the captured conversation exactly once", async () => {
    const props = headerProps();
    render(
      <TooltipProvider>
        <ConfigChatHeader {...props} />
      </TooltipProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: RESTART_LABEL }));
    fireEvent.click(screen.getByRole("button", { name: "common:cancel" }));
    expect(props.onRestart).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: RESTART_LABEL }));
    fireEvent.click(screen.getByTestId("config-chat-confirm-restart"));
    await waitFor(() => expect(props.onRestart).toHaveBeenCalledExactlyOnceWith(SESSION));
  });

  it("invalidates an open confirmation when its session is replaced", () => {
    const props = headerProps();
    const view = render(
      <TooltipProvider>
        <ConfigChatHeader {...props} />
      </TooltipProvider>,
    );
    fireEvent.click(screen.getByRole("button", { name: RESTART_LABEL }));
    view.rerender(
      <TooltipProvider>
        <ConfigChatHeader
          {...props}
          session={{ ...SESSION, sessionId: "session-2", taskId: "task-2" }}
        />
      </TooltipProvider>,
    );
    expect(screen.queryByTestId("config-chat-confirm-restart")).toBeNull();
    expect(props.onRestart).not.toHaveBeenCalled();
  });

  it("keeps Close available while restart and expansion are disabled", () => {
    const props = headerProps();
    render(
      <TooltipProvider>
        <ConfigChatHeader {...props} busy />
      </TooltipProvider>,
    );
    expect(
      (screen.getByRole("button", { name: RESTART_LABEL }) as HTMLButtonElement).disabled,
    ).toBe(true);
    expect(
      (screen.getByRole("button", { name: "configChat:openInQuickChat" }) as HTMLButtonElement)
        .disabled,
    ).toBe(true);
    fireEvent.click(screen.getByRole("button", { name: "configChat:closePanel" }));
    expect(props.onClose).toHaveBeenCalledOnce();
  });
});
