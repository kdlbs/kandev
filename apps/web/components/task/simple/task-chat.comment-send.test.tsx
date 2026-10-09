import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  createEvent,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { Toaster, toast } from "sonner";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { ActiveSessionRefProvider } from "./components/active-session-ref-context";
import { TaskChat } from "./task-chat";

const TASK_ID = "comment-draft-task";
const ORIGINAL = "  First instructions\nwith detail  ";
const NEXT = "  Next instructions\nkeep this draft  ";
const SECOND_TASK_TEXT = "Second task instructions";
const ERROR_MESSAGE = "Comment request rejected";

function successResponse(taskId = TASK_ID) {
  return new Response(
    JSON.stringify({
      comment: {
        id: "created-comment",
        task_id: taskId,
        body: ORIGINAL.trim(),
        author_type: "user",
        author_id: "user-1",
        created_at: "2026-10-08T14:00:00Z",
      },
    }),
    { status: 201, headers: { "Content-Type": "application/json" } },
  );
}

const cleanupSettlers: Array<() => void> = [];

function deferred<T>(cleanupValue: T) {
  let resolvePromise!: (value: T) => void;
  let rejectPromise!: (reason: unknown) => void;
  let settled = false;
  const promise = new Promise<T>((resolve, reject) => {
    resolvePromise = resolve;
    rejectPromise = reject;
  });
  const resolve = (value: T) => {
    settled = true;
    resolvePromise(value);
  };
  const reject = (reason: unknown) => {
    settled = true;
    rejectPromise(reason);
  };
  cleanupSettlers.push(() => {
    if (!settled) resolve(cleanupValue);
  });
  return { promise, resolve, reject };
}

type CommentRequest = {
  path: string;
  init: RequestInit;
  response: ReturnType<typeof deferred<Response>>;
};

let requests: CommentRequest[];
let unexpectedRequests: string[];

beforeEach(() => {
  requests = [];
  unexpectedRequests = [];
  vi.stubGlobal(
    "fetch",
    vi.fn((input: RequestInfo | URL, init: RequestInit = {}) => {
      const path = new URL(String(input), "http://localhost").pathname;
      if (path === "/api/v1/system/logs/frontend-errors" && init.method === "POST") {
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      if (/^\/api\/v1\/office\/tasks\/[^/]+\/comments$/.test(path) && init.method === "POST") {
        const response = deferred(successResponse());
        requests.push({ path, init, response });
        return response.promise;
      }
      unexpectedRequests.push(`${init.method ?? "GET"} ${path}`);
      return Promise.reject(new Error(`Unexpected fetch: ${path}`));
    }),
  );
});

afterEach(async () => {
  await act(async () => {
    cleanupSettlers.splice(0).forEach((settle) => settle());
    toast.dismiss();
  });
  cleanup();
  vi.unstubAllGlobals();
  expect(unexpectedRequests).toEqual([]);
});

function mountComposer(taskId = TASK_ID) {
  const refreshed = vi.fn();
  const rendered = render(
    <StateProvider>
      <ToastProvider>
        <TooltipProvider>
          <ActiveSessionRefProvider>
            <TaskChat taskId={taskId} comments={[]} sessions={[]} onCommentsChanged={refreshed} />
          </ActiveSessionRefProvider>
        </TooltipProvider>
      </ToastProvider>
      <Toaster />
    </StateProvider>,
  );
  const root = within(rendered.container).getByTestId("task-chat-root");
  const textbox = within(root).getByRole("textbox") as HTMLTextAreaElement;
  const send = within(root).getAllByRole("button").at(-1) as HTMLButtonElement;
  return { root, textbox, send, refreshed };
}

type Composer = ReturnType<typeof mountComposer>;

function edit(composer: Composer, text: string) {
  fireEvent.change(composer.textbox, { target: { value: text } });
}

function send(composer: Composer, method: "button" | "keyboard" = "button") {
  if (method === "button") fireEvent.click(composer.send);
  else fireEvent.keyDown(composer.textbox, { key: "Enter" });
}

function expectRequest(index: number, text: string, taskId = TASK_ID) {
  expect(requests).toHaveLength(index + 1);
  const request = requests[index];
  expect(request.path).toBe(`/api/v1/office/tasks/${taskId}/comments`);
  expect(request.init.method).toBe("POST");
  expect(JSON.parse(String(request.init.body))).toEqual({ body: text.trim(), author_type: "user" });
  return request;
}

async function acknowledge(request: CommentRequest) {
  await act(async () => request.response.resolve(successResponse()));
}

async function fail(request: CommentRequest, kind: "HTTP" | "fetch") {
  await act(async () => {
    if (kind === "HTTP") {
      request.response.resolve(
        new Response(JSON.stringify({ error: ERROR_MESSAGE }), { status: 500 }),
      );
    } else {
      request.response.reject(new Error(ERROR_MESSAGE));
    }
  });
  expect(screen.getAllByText(ERROR_MESSAGE).length).toBeGreaterThan(0);
}

describe("TaskChat comment send draft preservation", () => {
  // @covers AC-TASKS-COMMENT-DRAFT-001.1, AC-TASKS-COMMENT-DRAFT-001.2, AC-TASKS-COMMENT-DRAFT-001.4, AC-TASKS-COMMENT-DRAFT-001.8
  it.each([
    ["next instructions", NEXT],
    ["multiline instructions", "Next line\n\nAnother line"],
    ["equal trimmed text", ORIGINAL.trim()],
    ["whitespace-only draft", "  \n  "],
    ["empty draft", ""],
  ])("preserves changed raw text after older success: %s", async (_name, next) => {
    const composer = mountComposer();
    edit(composer, ORIGINAL);
    send(composer);
    const request = expectRequest(0, ORIGINAL);
    expect(composer.send.disabled).toBe(true);
    expect(composer.textbox.disabled).toBe(false);
    edit(composer, next);
    await acknowledge(request);
    expect(composer.textbox.value).toBe(next);
    expect(composer.refreshed).toHaveBeenCalledTimes(1);
    expect(composer.send.disabled).toBe(!next.trim());
    expect(requests).toHaveLength(1);
  });

  // @covers AC-TASKS-COMMENT-DRAFT-001.2, AC-TASKS-COMMENT-DRAFT-001.4
  it("clears the unchanged acknowledged raw draft", async () => {
    const composer = mountComposer();
    edit(composer, ORIGINAL);
    send(composer);
    await acknowledge(expectRequest(0, ORIGINAL));
    expect(composer.textbox.value).toBe("");
    expect(composer.send.disabled).toBe(true);
    expect(composer.refreshed).toHaveBeenCalledTimes(1);
  });

  it("clears the exact submitted text restored before success", async () => {
    const composer = mountComposer();
    edit(composer, ORIGINAL);
    send(composer);
    const request = expectRequest(0, ORIGINAL);
    edit(composer, NEXT);
    edit(composer, ORIGINAL);
    await acknowledge(request);
    expect(composer.textbox.value).toBe("");
    expect(composer.refreshed).toHaveBeenCalledTimes(1);
  });
});

describe("TaskChat comment send failures", () => {
  // @covers AC-TASKS-COMMENT-DRAFT-001.3
  it.each([
    ["HTTP", ORIGINAL],
    ["HTTP", NEXT],
    ["fetch", ORIGINAL],
    ["fetch", NEXT],
  ] as const)("retains current text on rejected %s request: %s", async (kind, current) => {
    const composer = mountComposer();
    edit(composer, ORIGINAL);
    send(composer);
    const request = expectRequest(0, ORIGINAL);
    edit(composer, current);
    await fail(request, kind);
    expect(composer.textbox.value).toBe(current);
    expect(composer.send.disabled).toBe(false);
    expect(composer.refreshed).not.toHaveBeenCalled();
    expect(requests).toHaveLength(1);
  });

  // @covers AC-TASKS-COMMENT-DRAFT-001.3, AC-TASKS-COMMENT-DRAFT-001.6
  it("allows an explicit retry after failure", async () => {
    const composer = mountComposer();
    edit(composer, ORIGINAL);
    send(composer);
    edit(composer, NEXT);
    await fail(expectRequest(0, ORIGINAL), "HTTP");
    expect(composer.textbox.value).toBe(NEXT);
    send(composer);
    await acknowledge(expectRequest(1, NEXT));
    expect(composer.textbox.value).toBe("");
    expect(composer.refreshed).toHaveBeenCalledTimes(1);
  });
});

describe("TaskChat comment send admission", () => {
  // @covers AC-TASKS-COMMENT-DRAFT-001.4, AC-TASKS-COMMENT-DRAFT-001.5, AC-TASKS-COMMENT-DRAFT-001.8
  it.each(["button", "keyboard"] as const)("admits %s sends consistently", async (method) => {
    const composer = mountComposer();
    edit(composer, ORIGINAL);
    send(composer, method);
    const request = expectRequest(0, ORIGINAL);
    edit(composer, NEXT);
    send(composer, "keyboard");
    send(composer, "button");
    expect(requests).toHaveLength(1);
    expect(composer.send.disabled).toBe(true);
    expect(composer.textbox.disabled).toBe(false);
    await acknowledge(request);
    expect(composer.textbox.value).toBe(NEXT);
    expect(composer.send.disabled).toBe(false);
    expect(composer.refreshed).toHaveBeenCalledTimes(1);
  });

  it.each(["", " \n  "])("blocks empty and whitespace-only sends: %j", (text) => {
    const composer = mountComposer();
    edit(composer, text);
    send(composer, "keyboard");
    send(composer, "button");
    expect(requests).toHaveLength(0);
    expect(composer.send.disabled).toBe(true);
    expect(composer.refreshed).not.toHaveBeenCalled();
  });

  it("keeps Shift Enter available without submitting", () => {
    const composer = mountComposer();
    edit(composer, ORIGINAL);
    const event = createEvent.keyDown(composer.textbox, { key: "Enter", shiftKey: true });
    fireEvent(composer.textbox, event);
    expect(event.defaultPrevented).toBe(false);
    expect(requests).toHaveLength(0);
    expect(composer.textbox.value).toBe(ORIGINAL);
    expect(composer.refreshed).not.toHaveBeenCalled();
  });

  // @covers AC-TASKS-COMMENT-DRAFT-001.6
  it("sends the retained next draft on a later deliberate send", async () => {
    const composer = mountComposer();
    edit(composer, ORIGINAL);
    send(composer);
    edit(composer, NEXT);
    await acknowledge(expectRequest(0, ORIGINAL));
    send(composer, "keyboard");
    await acknowledge(expectRequest(1, NEXT));
    expect(composer.textbox.value).toBe("");
    expect(composer.refreshed).toHaveBeenCalledTimes(2);
  });
});

describe("TaskChat comment composer independence", () => {
  // @covers AC-TASKS-COMMENT-DRAFT-001.7
  it.each(["success", "failure"] as const)(
    "keeps mounted task composers isolated: %s",
    async (outcome) => {
      const first = mountComposer("first-task");
      const second = mountComposer("second-task");
      edit(first, ORIGINAL);
      send(first);
      const firstRequest = expectRequest(0, ORIGINAL, "first-task");
      edit(first, NEXT);
      edit(second, SECOND_TASK_TEXT);
      expect(second.send.disabled).toBe(false);
      send(second);
      const secondRequest = expectRequest(1, SECOND_TASK_TEXT, "second-task");
      if (outcome === "success") await acknowledge(secondRequest);
      else await fail(secondRequest, "fetch");
      expect(second.textbox.value).toBe(outcome === "success" ? "" : SECOND_TASK_TEXT);
      expect(second.refreshed).toHaveBeenCalledTimes(outcome === "success" ? 1 : 0);
      expect(first.send.disabled).toBe(true);
      expect(first.textbox.value).toBe(NEXT);
      expect(first.refreshed).not.toHaveBeenCalled();
      await acknowledge(firstRequest);
      expect(first.textbox.value).toBe(NEXT);
      expect(first.refreshed).toHaveBeenCalledTimes(1);
      expect(second.textbox.value).toBe(outcome === "success" ? "" : SECOND_TASK_TEXT);
      expect(second.send.disabled).toBe(outcome === "success");
    },
  );

  // @covers AC-TASKS-COMMENT-DRAFT-001.1
  it("preserves asynchronously inserted file text before acknowledgement", async () => {
    const composer = mountComposer();
    edit(composer, ORIGINAL);
    send(composer);
    const request = expectRequest(0, ORIGINAL);
    const fileText = deferred("Attachment instructions");
    const file = new File([], "instructions.txt", { type: "text/plain" });
    Object.defineProperty(file, "text", { value: () => fileText.promise });
    const fileInput = composer.root.querySelector('input[type="file"]') as HTMLInputElement;
    fireEvent.change(fileInput, { target: { files: [file] } });
    await act(async () => fileText.resolve("Attachment instructions"));
    const inserted = `${ORIGINAL}\n\n**instructions.txt**\n\`\`\`\nAttachment instructions\n\`\`\``;
    expect(composer.textbox.value).toBe(inserted);
    await acknowledge(request);
    expect(composer.textbox.value).toBe(inserted);
    expect(composer.refreshed).toHaveBeenCalledTimes(1);
  });
});
