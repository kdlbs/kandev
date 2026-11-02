import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import { StateProvider } from "@/components/state-provider";
import { entityReferenceMarkdown } from "@/lib/entity-references/message-references";
import type { EntityReference } from "@/lib/types/entity-reference";
import { usePromptMentionMarkdownComponents } from "./prompt-mention-components";
import { renderUserMessageBody } from "./user-message-body";

afterEach(cleanup);

const TASK_CHIP_TEST_ID = "task-mention-chip";
const FORMATTED_MENTION = "@Fix **retry** loop";

const tasks = [
  { title: "Retry", taskId: "recorded-task" },
  { title: "Retry capture", taskId: "longer-task" },
  { title: "修复", taskId: "unicode-task" },
  { title: "Fix **retry** loop", taskId: "formatted-task" },
  { title: "Fix `retry` loop", taskId: "code-title-task" },
  { title: "Fix [retry](https://example.com) loop", taskId: "link-title-task" },
  { title: "Fix :bug: retry", taskId: "emoji-title-task" },
];

function Message({
  content,
  raw = false,
  references = [],
}: {
  content: string;
  raw?: boolean;
  references?: EntityReference[];
}) {
  const components = usePromptMentionMarkdownComponents(["Retry", "daily"], references);
  return renderUserMessageBody({
    content,
    hasContent: true,
    hasAttachments: false,
    showRaw: raw,
    taskId: "source-task",
    promptMentionComponents: components,
    taskMentions: tasks,
  });
}

function renderMessage(content: string, raw = false, references: EntityReference[] = []) {
  return render(
    <StateProvider initialState={{ prompts: { items: [], loaded: true, loading: false } }}>
      <ToastProvider>
        <Message content={content} raw={raw} references={references} />
      </ToastProvider>
    </StateProvider>,
  );
}

// @covers AC-UI-CHAT-TASK-MENTION-001.1
// @covers AC-UI-CHAT-TASK-MENTION-001.2
// @covers AC-UI-CHAT-TASK-MENTION-001.3
describe("formatted task mentions", () => {
  it("uses normalized fence boundaries before protecting task titles", () => {
    renderMessage("```text\nlog```\n\n@Fix **retry** loop");
    expect(screen.getByRole("link", { name: FORMATTED_MENTION }).getAttribute("href")).toBe(
      "/t/formatted-task",
    );
  });

  it("keeps Markdown characters inside recorded task titles literal and clickable", () => {
    renderMessage(
      "Check @Fix **retry** loop and @Fix `retry` loop and @Fix [retry](https://example.com) loop and @Fix :bug: retry.",
    );
    for (const task of tasks.slice(3)) {
      expect(screen.getByRole("link", { name: `@${task.title}` }).getAttribute("href")).toBe(
        `/t/${task.taskId}`,
      );
    }
  });

  it("preserves code and existing links containing formatted task titles", () => {
    const { container } = renderMessage(
      "`@Fix **retry** loop` [@Fix **retry** loop](/docs)\n\n@Fix **retry** loop",
    );
    expect(screen.getAllByTestId(TASK_CHIP_TEST_ID)).toHaveLength(1);
    expect(container.querySelector('a[href="/docs"] strong')?.textContent).toBe("retry");
    expect(container.querySelector("code")?.textContent).toBe(FORMATTED_MENTION);
    expect(container.querySelectorAll("a a")).toHaveLength(0);
  });
  it("retains adjacent-word boundaries after protecting a formatted task title", () => {
    const first = renderMessage("@Fix **retry** loop@daily");
    expect(screen.getAllByTestId(TASK_CHIP_TEST_ID)).toHaveLength(1);
    expect(screen.queryByTestId("custom-prompt-mention")).toBeNull();
    first.unmount();
    renderMessage("@Fix **retry** loop @daily");
    expect(screen.getByTestId("custom-prompt-mention").textContent).toBe("@daily");
  });
});

describe("composed user-message task mentions", () => {
  it("renders task references independently of saved prompt renderers", () => {
    render(
      <StateProvider initialState={{ prompts: { items: [], loaded: true, loading: false } }}>
        <ToastProvider>
          {renderUserMessageBody({
            content: "@Retry",
            hasContent: true,
            hasAttachments: false,
            showRaw: false,
            taskId: "source-task",
            taskMentions: [tasks[0]],
          })}
        </ToastProvider>
      </StateProvider>,
    );
    expect(screen.getByRole("link", { name: "@Retry" }).getAttribute("href")).toBe(
      "/t/recorded-task",
    );
  });

  it("uses message task identity ahead of colliding prompt names and matches longest titles", () => {
    renderMessage("Check @Retry capture and **@Retry** plus @daily and @修复. @ordinary");
    expect(screen.getByRole("link", { name: "@Retry capture" }).getAttribute("href")).toBe(
      "/t/longer-task",
    );
    expect(screen.getByRole("link", { name: "@Retry" }).getAttribute("href")).toBe(
      "/t/recorded-task",
    );
    expect(screen.getByRole("link", { name: "@修复" }).getAttribute("href")).toBe(
      "/t/unicode-task",
    );
    expect(screen.getByTestId("custom-prompt-mention").textContent).toBe("@daily");
    expect(screen.queryByRole("link", { name: "@ordinary" })).toBeNull();
  });

  it("keeps code and link labels intact and does not cross Unicode or sibling boundaries", () => {
    const { container } = renderMessage(
      "`@Retry` [@Retry](/docs) word**@Retry** @Retrying @修复中\n\n```\n@Retry\n```\n\n@Retry",
    );
    expect(screen.getAllByTestId(TASK_CHIP_TEST_ID)).toHaveLength(1);
    expect(container.querySelectorAll("a a")).toHaveLength(0);
    expect(container.querySelector("code")?.textContent).toBe("@Retry");
    expect(container.querySelector('a[href="/docs"]')?.textContent).toBe("@Retry");
  });

  it("does not decorate raw content", () => {
    const { container } = renderMessage("@Fix **retry** loop", true);
    expect(screen.queryByTestId(TASK_CHIP_TEST_ID)).toBeNull();
    expect(container.querySelector("pre")?.textContent).toBe(FORMATTED_MENTION);
  });

  it("keeps existing external entity links alongside task chips", () => {
    const reference: EntityReference = {
      version: 1,
      ref: "mention:v1:linear:issue:acme:one",
      provider: "linear",
      kind: "issue",
      scope: "acme",
      id: "one",
      key: "ENG-1",
      title: "Fix capture",
      url: "https://example.com/ENG-1",
    };
    renderMessage(`@Fix **retry** loop ${entityReferenceMarkdown(reference)}`, false, [reference]);
    expect(screen.getByTestId(TASK_CHIP_TEST_ID).getAttribute("href")).toBe("/t/formatted-task");
    expect(screen.getByTestId("entity-reference-chip").getAttribute("href")).toBe(reference.url);
  });

  it("decorates only the bounded preview without revealing the hidden tail", () => {
    const { container } = renderMessage(
      `@Fix **retry** loop\n${"line\n".repeat(250)}@Retry capture`,
    );
    expect(screen.getAllByTestId(TASK_CHIP_TEST_ID)).toHaveLength(1);
    expect(container.textContent).not.toContain("@Retry capture");
  });
});
