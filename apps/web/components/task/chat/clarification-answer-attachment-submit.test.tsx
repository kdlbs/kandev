import { createRef } from "react";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Message } from "@/lib/types/http";
import { ClarificationInputOverlay } from "./clarification-input-overlay";
import type { FileAttachment } from "./file-attachment";

const API = vi.hoisted(() => ({
  processFile: vi.fn(),
  upload: vi.fn(),
  fetchTask: vi.fn(),
  updateMessage: vi.fn(),
}));

vi.mock("@/lib/config", () => ({
  getBackendConfig: () => ({ apiBaseUrl: "https://api.test" }),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => ({ getState: () => ({ updateMessage: API.updateMessage }) }),
}));

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

vi.mock("@/lib/api/domains/kanban-api", () => ({
  fetchTask: (...args: unknown[]) => API.fetchTask(...args),
}));

vi.mock("@/lib/api/domains/attachment-api", () => ({
  uploadAttachment: (...args: unknown[]) => API.upload(...args),
  deleteAttachment: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("./file-attachment", async () => {
  const actual = await vi.importActual<typeof import("./file-attachment")>("./file-attachment");
  return { ...actual, processFile: (...args: unknown[]) => API.processFile(...args) };
});

function message(): Message {
  return {
    id: "m1",
    session_id: "session-1",
    task_id: "task-1",
    author_type: "agent",
    content: "Q",
    type: "clarification_request",
    created_at: "2026-05-04T00:00:00Z",
    metadata: {
      pending_id: "pending-submit-gate",
      question_id: "q1",
      question_index: 0,
      question_total: 1,
      status: "pending",
      question: {
        id: "q1",
        title: "",
        prompt: "Question?",
        options: [{ option_id: "o1", label: "Option 1", description: "" }],
      },
    },
  } as unknown as Message;
}

function processed(name: string): FileAttachment {
  return {
    id: name,
    file: new File([new Uint8Array([1])], name, { type: "image/png" }),
    mimeType: "image/png",
    fileName: name,
    size: 1,
    isImage: true,
    deliveryMode: "path",
    uploadStatus: "pending",
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise;
  });
  return { promise, resolve };
}

const fetchMock = vi.fn();

beforeEach(() => {
  API.processFile.mockReset();
  API.upload.mockReset();
  API.fetchTask.mockReset();
  API.updateMessage.mockReset();
  API.fetchTask.mockResolvedValue({ workspace_id: "workspace-1" });
  API.upload.mockResolvedValue({
    attachment_id: "att-1",
    name: "a.png",
    mime_type: "image/png",
    kind: "image",
    delivery_mode: "path",
    size_bytes: 1,
  });
  fetchMock.mockReset();
  fetchMock.mockResolvedValue(
    new Response(JSON.stringify({ success: true, claimed: true, status: "answered" }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }),
  );
  globalThis.fetch = fetchMock as unknown as typeof globalThis.fetch;
});

afterEach(cleanup);

describe("ClarificationInputOverlay attachment submit gate", () => {
  it("does not submit while a picked file is still being processed", async () => {
    const processing = deferred<FileAttachment>();
    API.processFile.mockImplementationOnce(() => processing.promise);
    const scopeRef = createRef<HTMLDivElement>();
    const { container } = render(
      <div ref={scopeRef} tabIndex={-1}>
        <ClarificationInputOverlay
          messages={[message()]}
          onResolved={vi.fn()}
          onDismiss={vi.fn()}
          shortcutScopeRef={scopeRef}
        />
      </div>,
    );

    fireEvent.click(screen.getByTestId("clarification-option"));
    const input = container.querySelector<HTMLInputElement>(
      '[data-testid="clarification-attachment-input"]',
    );
    expect(input).not.toBeNull();
    fireEvent.change(input!, {
      target: { files: [new File([new Uint8Array([1])], "a.png", { type: "image/png" })] },
    });

    const send = screen.getByTestId("clarification-custom-submit") as HTMLButtonElement;
    expect(send.disabled).toBe(true);
    fireEvent.click(send);
    expect(fetchMock).not.toHaveBeenCalled();

    processing.resolve(processed("a.png"));
    await waitFor(() => expect(send.disabled).toBe(false));
    fireEvent.click(send);
    await waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
  });
});
