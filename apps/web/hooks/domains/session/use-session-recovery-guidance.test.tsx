import { Suspense, type ReactNode } from "react";
import { act, render } from "@testing-library/react";
import { expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { sessionId, taskId, type TaskSession } from "@/lib/types/http";
import { useSessionRecoveryGuidance } from "./use-session-recovery-guidance";

const session: TaskSession = {
  id: sessionId("session"),
  task_id: taskId("task"),
  state: "WAITING_FOR_INPUT",
  started_at: "2026-10-15T10:00:00Z",
  updated_at: "2026-10-15T10:00:00Z",
};

it("keeps the committed identity when a render with a different incident suspends", () => {
  let shouldSuspend = false;
  let observedRequestKey: string | undefined;
  const suspended = new Promise<void>(() => {});
  const Probe = ({ errorStamp }: { errorStamp: string }) => {
    const { identityRef } = useSessionRecoveryGuidance("task", "session", session, errorStamp);
    observedRequestKey = identityRef.current.requestKey;
    if (shouldSuspend) throw suspended;
    return null;
  };
  const wrapper = ({ children }: { children: ReactNode }) => (
    <StateProvider initialState={{ taskSessions: { items: { [session.id]: session } } }}>
      <Suspense fallback={null}>{children}</Suspense>
    </StateProvider>
  );
  const view = render(<Probe errorStamp="incident-a" />, { wrapper });
  const committedRequestKey = observedRequestKey;

  shouldSuspend = true;
  act(() => view.rerender(<Probe errorStamp="incident-b" />));

  expect(committedRequestKey).toContain("incident-a");
  expect(observedRequestKey).toBe(committedRequestKey);
});
