import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act, waitFor, type RenderHookResult } from "@testing-library/react";
import { sessionId as toSessionId } from "@/lib/types/http";
import type { Message } from "@/lib/types/http";
import {
  parsePermission,
  usePermissionResponseHandlers,
  type PermissionOption,
  type PermissionRequestMetadata,
} from "./use-permission-handlers";

const mocks = vi.hoisted(() => ({
  toastError: vi.fn(),
  toastWarning: vi.fn(),
}));
const requestMock = vi.fn().mockResolvedValue({});

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: requestMock }),
}));
vi.mock("@/lib/toast/sonner", () => ({
  toast: { error: mocks.toastError, warning: mocks.toastWarning },
}));
beforeEach(() => {
  requestMock.mockReset().mockResolvedValue({});
  mocks.toastError.mockReset();
  mocks.toastWarning.mockReset();
});

const ALLOW_ONCE: PermissionOption = { option_id: "allow", name: "Allow", kind: "allow_once" };
const REJECT_ONCE: PermissionOption = { option_id: "deny", name: "Deny", kind: "reject_once" };
const ALLOW_ALWAYS: PermissionOption = {
  option_id: "always",
  name: "Allow always",
  kind: "allow_always",
};

function makePermissionMessage(options: PermissionOption[] = [ALLOW_ONCE, REJECT_ONCE]): Message {
  const meta: PermissionRequestMetadata = {
    request_id: "req-1",
    pending_id: "pend-1",
    tool_call_id: "tc-1",
    action_type: "mcp_tool",
    action_details: {},
    options,
  };
  return {
    id: "msg-1",
    session_id: toSessionId("sess-1"),
    task_id: "task-1" as ReturnType<typeof import("@/lib/types/http").taskId>,
    author_type: "agent",
    content: "",
    type: "permission_request",
    created_at: "2026-05-25T00:00:00Z",
    metadata: meta as unknown as Record<string, unknown>,
  } as unknown as Message;
}

type Handlers = ReturnType<typeof usePermissionResponseHandlers>;

function renderHandlers(message: Message): RenderHookResult<Handlers, unknown>["result"] {
  return renderHook(() =>
    usePermissionResponseHandlers({
      permissionMetadata: message.metadata as unknown as PermissionRequestMetadata,
      permissionMessage: message,
    }),
  ).result;
}

function firstPayload(): Record<string, unknown> {
  return requestMock.mock.calls[0][1] as Record<string, unknown>;
}

describe("handleApprove", () => {
  it("sends cancelled=false and rejected=false", async () => {
    const result = renderHandlers(makePermissionMessage());
    await act(async () => {
      result.current.handleApprove();
    });

    expect(requestMock).toHaveBeenCalledOnce();
    expect(firstPayload().option_id).toBe("allow");
    expect(firstPayload().task_id).toBe("task-1");
    expect(firstPayload().session_id).toBe("sess-1");
    expect(firstPayload().request_id).toBe("req-1");
    expect(firstPayload().pending_id).toBe("pend-1");
    expect(firstPayload().cancelled).toBeFalsy();
    expect(firstPayload().rejected).toBeFalsy();
  });

  it("prefers allow_once over allow_always so 'Always allow' stays distinct", async () => {
    const once: PermissionOption = { option_id: "once", name: "Allow once", kind: "allow_once" };
    const result = renderHandlers(makePermissionMessage([ALLOW_ALWAYS, once, REJECT_ONCE]));
    await act(async () => {
      result.current.handleApprove();
    });

    expect(firstPayload().option_id).toBe("once");
  });

  it("falls back to allow_always when no allow_once is offered", async () => {
    const result = renderHandlers(makePermissionMessage([ALLOW_ALWAYS]));
    await act(async () => {
      result.current.handleApprove();
    });

    expect(firstPayload().option_id).toBe("always");
  });
});

describe("request identity", () => {
  it("treats a cached pre-generation request as expired and unanswerable", async () => {
    const message = makePermissionMessage();
    delete (message.metadata as PermissionRequestMetadata).request_id;
    expect(parsePermission(message)).toMatchObject({
      permissionStatus: "expired",
      isPermissionPending: false,
    });
    const result = renderHandlers(message);

    await act(async () => {
      result.current.handleApprove();
    });

    expect(requestMock).not.toHaveBeenCalled();
    expect(result.current.isUnavailable).toBe(true);
    expect(mocks.toastWarning).toHaveBeenCalledWith(
      "This permission request is no longer available.",
    );
  });

  it("expires the cached card and shows feedback when the executor response is stale", async () => {
    const message = makePermissionMessage();
    requestMock.mockRejectedValueOnce(new Error("permission_not_found"));
    const result = renderHandlers(message);

    await act(async () => {
      result.current.handleApprove();
    });

    await waitFor(() => expect(result.current.isUnavailable).toBe(true));
    expect(mocks.toastWarning).toHaveBeenCalledWith(
      "This permission request is no longer available.",
    );
  });

  it("resets unavailable state when a replacement generation arrives", async () => {
    const first = makePermissionMessage();
    requestMock.mockRejectedValueOnce(new Error("permission_not_found"));
    const rendered = renderHook(
      ({ message }: { message: Message }) =>
        usePermissionResponseHandlers({
          permissionMetadata: message.metadata as unknown as PermissionRequestMetadata,
          permissionMessage: message,
        }),
      { initialProps: { message: first } },
    );

    await act(async () => {
      rendered.result.current.handleApprove();
    });
    await waitFor(() => expect(rendered.result.current.isUnavailable).toBe(true));

    const replacement = makePermissionMessage();
    const replacementMetadata = replacement.metadata as unknown as PermissionRequestMetadata;
    replacementMetadata.request_id = "req-2";
    replacementMetadata.pending_id = "pend-2";
    rendered.rerender({ message: replacement });

    await waitFor(() => expect(rendered.result.current.isUnavailable).toBe(false));
    await act(async () => {
      rendered.result.current.handleApprove();
    });

    expect(requestMock).toHaveBeenCalledTimes(2);
    expect(requestMock.mock.calls[1][1]).toMatchObject({
      request_id: "req-2",
      pending_id: "pend-2",
    });
  });

  it("ignores a stale response failure after a replacement generation arrives", async () => {
    const first = makePermissionMessage();
    let rejectFirst!: (reason?: unknown) => void;
    requestMock.mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          rejectFirst = reject;
        }),
    );
    const rendered = renderHook(
      ({ message }: { message: Message }) =>
        usePermissionResponseHandlers({
          permissionMetadata: message.metadata as unknown as PermissionRequestMetadata,
          permissionMessage: message,
        }),
      { initialProps: { message: first } },
    );

    await act(async () => {
      rendered.result.current.handleApprove();
    });
    await waitFor(() => expect(rejectFirst).toBeTypeOf("function"));

    const replacement = makePermissionMessage();
    const replacementMetadata = replacement.metadata as unknown as PermissionRequestMetadata;
    replacementMetadata.request_id = "req-2";
    replacementMetadata.pending_id = "pend-2";
    rendered.rerender({ message: replacement });
    await waitFor(() => expect(rendered.result.current.isUnavailable).toBe(false));

    await act(async () => {
      rejectFirst(new Error("permission_not_found"));
    });

    expect(rendered.result.current.isUnavailable).toBe(false);
    expect(mocks.toastWarning).not.toHaveBeenCalled();
  });
});

describe("handleAllowAlways", () => {
  it("hasAllowAlways is true and responds with the allow_always option (not rejected)", async () => {
    const result = renderHandlers(makePermissionMessage([ALLOW_ONCE, ALLOW_ALWAYS, REJECT_ONCE]));
    expect(result.current.hasAllowAlways).toBe(true);

    await act(async () => {
      result.current.handleAllowAlways();
    });

    expect(requestMock).toHaveBeenCalledOnce();
    expect(firstPayload().option_id).toBe("always");
    expect(firstPayload().cancelled).toBeFalsy();
    expect(firstPayload().rejected).toBeFalsy();
  });

  it("hasAllowAlways is false and handleAllowAlways is a no-op when not offered", async () => {
    const result = renderHandlers(makePermissionMessage([ALLOW_ONCE, REJECT_ONCE]));
    expect(result.current.hasAllowAlways).toBe(false);

    await act(async () => {
      result.current.handleAllowAlways();
    });

    expect(requestMock).not.toHaveBeenCalled();
  });
});

describe("handleReject", () => {
  it("sends rejected=true and cancelled=false when a reject option exists", async () => {
    const result = renderHandlers(makePermissionMessage());
    await act(async () => {
      result.current.handleReject();
    });

    expect(requestMock).toHaveBeenCalledOnce();
    expect(firstPayload().option_id).toBe("deny");
    expect(firstPayload().rejected).toBe(true);
    // Must NOT set cancelled=true: that triggers EventTypePermissionCancelled
    // which races against the orchestrator's UpdatePermissionMessage("rejected")
    // and would overwrite the status to "expired".
    expect(firstPayload().cancelled).toBeFalsy();
  });

  it("falls back to cancelled=true when no reject option exists", async () => {
    const result = renderHandlers(makePermissionMessage([ALLOW_ONCE]));
    await act(async () => {
      result.current.handleReject();
    });

    expect(requestMock).toHaveBeenCalledOnce();
    expect(firstPayload().cancelled).toBe(true);
    expect(firstPayload().option_id).toBeUndefined();
  });
});
