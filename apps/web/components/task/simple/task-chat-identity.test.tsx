import { afterEach, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import { ChatIdentityContext, PersonaIdentityContext } from "./persona-identity-context";
import { ActiveSessionRefProvider } from "./components/active-session-ref-context";

vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: vi.fn() }) }));
vi.mock("@/hooks/use-is-utility-configured", () => ({ useIsUtilityConfigured: () => true }));
vi.mock("@/hooks/use-utility-agent-generator", () => ({
  useUtilityAgentGenerator: () => ({ enhancePrompt: vi.fn(), isEnhancingPrompt: false }),
}));
vi.mock("./markdown-comment", () => ({
  MarkdownComment: ({ content }: { content: string }) => <p>{content}</p>,
}));
vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
import { TaskChat } from "./task-chat";

afterEach(cleanup);
function wrap(children: ReactNode) {
  return (
    <StateProvider>
      <ActiveSessionRefProvider>{children}</ActiveSessionRefProvider>
    </StateProvider>
  );
}

it("resolves feature-provided agent names and prefers the current persona identity", () => {
  const comment = {
    id: "identity",
    taskId: "task-1",
    content: "A generic project update.",
    createdAt: "2026-05-01T10:00:00Z",
    authorType: "agent" as const,
    authorId: "chief",
    authorName: "",
  };
  const view = (name: string | null) =>
    wrap(
      <ChatIdentityContext.Provider value={{ chief: "Office assistant" }}>
        <PersonaIdentityContext.Provider value={name ? { id: "chief", name } : null}>
          <TaskChat taskId="task-1" comments={[comment]} sessions={[]} readOnly />
        </PersonaIdentityContext.Provider>
      </ChatIdentityContext.Provider>,
    );
  const { rerender } = render(view(null));
  expect(screen.getByText("Office assistant")).not.toBeNull();
  rerender(view("Chief of staff"));
  expect(screen.getByText("Chief of staff")).not.toBeNull();
  expect(screen.queryByText("Office assistant")).toBeNull();
});
