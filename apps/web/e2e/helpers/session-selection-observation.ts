import type { Page } from "@playwright/test";

const lifecycleActions = new Set([
  "session.stop",
  "session.launch",
  "session.resume",
  "session.restart",
  "session.delete",
  "session.set_primary",
]);

type LifecycleRequest = {
  type?: string;
  action: string;
  payload?: Record<string, unknown>;
};

/** Retain complete outgoing lifecycle payloads, including unexpected fields. */
export function observeSessionSelection(page: Page) {
  const requests: LifecycleRequest[] = [];
  page.on("websocket", (socket) => {
    if (!socket.url().endsWith("/ws")) return;
    socket.on("framesent", ({ payload }) => {
      try {
        const message = JSON.parse(payload.toString()) as LifecycleRequest;
        if (lifecycleActions.has(message.action)) requests.push(message);
      } catch {
        // Non-JSON socket traffic is not a session lifecycle request.
      }
    });
  });
  return requests;
}

/** Only the existing promptless focus signal for this live fixture is expected. */
export function unexpectedSelectionRequests(
  requests: readonly LifecycleRequest[],
  taskId: string,
  liveSessionId: string,
) {
  return requests.filter((request) => {
    const payload = request.payload;
    return !(
      request.type === "request" &&
      request.action === "session.launch" &&
      payload &&
      Object.keys(payload).sort().join(",") === "activation_source,intent,session_id,task_id" &&
      payload.task_id === taskId &&
      payload.session_id === liveSessionId &&
      payload.intent === "resume" &&
      payload.activation_source === "session_focus"
    );
  });
}

export type SelectionToast = { title: string; description: string; icons: string[] };

/** The background update hook emits this default informational notice. */
export function isBackgroundUpdateNotice(toast: SelectionToast) {
  return (
    toast.title === "Kandev update available" &&
    /^Kandev \S+ is available\. Open Settings > System > Updates to review it\.$/.test(
      toast.description,
    ) &&
    toast.icons.length === 1 &&
    toast.icons[0] === "tabler-icon tabler-icon-info-circle h-5 w-5"
  );
}

/** Read actual rendered feedback without dismissing or muting it. */
export async function readSelectionToasts(page: Page): Promise<SelectionToast[]> {
  return page.getByTestId("toast-message").evaluateAll((toasts) =>
    toasts.map((toast) => ({
      title: toast.querySelector(".font-semibold")?.textContent ?? "",
      description: toast.querySelector(".text-xs")?.textContent ?? "",
      icons: Array.from(toast.querySelectorAll("svg"), (icon) => icon.getAttribute("class") ?? ""),
    })),
  );
}
