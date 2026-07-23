import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueuedGhostMessage } from "./queued-ghost-message";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { entityReferenceMarkdown } from "@/lib/entity-references/message-references";
import type { EntityReference } from "@/lib/types/entity-reference";
import type { QueuedMessage } from "@/lib/state/slices/session/types";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});

vi.mock("@kandev/ui/tooltip", () => ({
  Tooltip: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipTrigger: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipContent: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  TooltipProvider: ({ children }: { children: React.ReactNode }) => <>{children}</>,
}));

const PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=";

const ATTACHMENT_1_ALT = "Attachment 1";
const OPEN_ATTACHMENT_1_LABEL = "Open Attachment 1";
const FULL_SIZE_ATTACHMENT_1_ALT = "Full size Attachment 1";

function entry(overrides: Partial<QueuedMessage> = {}): QueuedMessage {
  return {
    id: "q-1",
    session_id: "sess-1",
    task_id: "task-1",
    content: "hello",
    plan_mode: false,
    queued_at: "2026-05-18T00:00:00Z",
    queued_by: "user-1",
    ...overrides,
  };
}

function taskReference(overrides: Partial<EntityReference> = {}): EntityReference {
  return {
    version: 1,
    ref: "mention:v1:kandev:task:workspace-1:task-1",
    provider: "kandev",
    kind: "task",
    id: "task-1",
    key: "KAN-1",
    title: "Fix authentication",
    url: "/t/task-1",
    scope: "workspace-1",
    ...overrides,
  };
}

function issueReference(): EntityReference {
  return {
    version: 1,
    ref: "mention:v1:jira:issue:https%3A%2F%2Fjira.example:10001",
    provider: "jira",
    kind: "issue",
    id: "10001",
    key: "ENG-1",
    title: "Fix login flow",
    url: "https://jira.example/browse/ENG-1",
    scope: "https://jira.example",
  };
}

function renderWithProviders(node: React.ReactNode) {
  return render(
    <StateProvider>
      <ToastProvider>{node}</ToastProvider>
    </StateProvider>,
  );
}

describe("QueuedGhostMessage workflow badge", () => {
  it("renders workflow metadata as a workflow step badge", () => {
    render(
      <QueuedGhostMessage
        entry={entry({
          queued_by: "workflow",
          metadata: {
            workflow_message: true,
            workflow_step_name: "In Progress",
            workflow_step_color: "bg-green-500",
          },
        })}
        canEdit={false}
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );

    expect(screen.getByTestId("workflow-message-badge").textContent).toContain("In Progress");
    expect(screen.getByTestId("workflow-message-dot").className).toContain("bg-green-500");
    expect(screen.queryByTestId("sender-task-badge")).toBeNull();
  });
});

describe("QueuedGhostMessage attachment thumbnails", () => {
  it("renders an image thumbnail for image attachments", () => {
    render(
      <QueuedGhostMessage
        entry={entry({
          attachments: [{ type: "image", data: PNG_BASE64, mime_type: "image/png" }],
        })}
        canEdit
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );
    const trigger = screen.getByRole("button", { name: OPEN_ATTACHMENT_1_LABEL });
    const img = trigger.querySelector("img") as HTMLImageElement;
    expect(img.src).toBe(`data:image/png;base64,${PNG_BASE64}`);
    expect(trigger.className).toContain("cursor-pointer");
  });

  it("renders a file chip for non-image (resource) attachments", () => {
    render(
      <QueuedGhostMessage
        entry={entry({
          content: "",
          attachments: [{ type: "resource", data: "ZmlsZQ==", mime_type: "text/plain" }],
        })}
        canEdit
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );
    expect(screen.getByText("Attachment")).toBeTruthy();
  });

  it("renders image thumbnails as accessible dialog triggers in display mode", () => {
    render(
      <QueuedGhostMessage
        entry={entry({
          attachments: [{ type: "image", data: PNG_BASE64, mime_type: "image/png" }],
        })}
        canEdit
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );
    const trigger = screen.getByRole("button", { name: OPEN_ATTACHMENT_1_LABEL });
    expect(trigger.getAttribute("type")).toBe("button");
    expect(trigger.querySelector("img")).toBeTruthy();
  });

  it("opens the image in a preview dialog when clicked in display mode", () => {
    const openSpy = vi.spyOn(window, "open").mockReturnValue(null);
    render(
      <QueuedGhostMessage
        entry={entry({
          attachments: [{ type: "image", data: PNG_BASE64, mime_type: "image/png" }],
        })}
        canEdit
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: OPEN_ATTACHMENT_1_LABEL }));
    expect(openSpy).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeTruthy();
    expect(screen.getByAltText(FULL_SIZE_ATTACHMENT_1_ALT).getAttribute("src")).toBe(
      `data:image/png;base64,${PNG_BASE64}`,
    );
  });

  it("renders thumbnails read-only in edit mode (no dialog trigger, no cursor-pointer)", () => {
    const openSpy = vi.spyOn(window, "open").mockReturnValue(null);
    render(
      <QueuedGhostMessage
        entry={entry({
          attachments: [{ type: "image", data: PNG_BASE64, mime_type: "image/png" }],
        })}
        canEdit
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );
    fireEvent.click(screen.getByTitle("Edit queued message"));
    const img = screen.getByAltText(ATTACHMENT_1_ALT) as HTMLImageElement;
    expect(img.className).not.toContain("cursor-pointer");
    expect(screen.queryByRole("button", { name: OPEN_ATTACHMENT_1_LABEL })).toBeNull();
    fireEvent.click(img);
    expect(openSpy).not.toHaveBeenCalled();
  });

  it("renders no thumbnail row when there are no attachments", () => {
    const { container } = render(
      <QueuedGhostMessage entry={entry()} canEdit onSave={async () => {}} onRemove={() => {}} />,
    );
    expect(container.querySelector("img")).toBeNull();
  });
});

describe("QueuedGhostMessage entity references", () => {
  it("renders an exact queued Markdown link as an accessible chip", () => {
    const reference = taskReference();

    renderWithProviders(
      <QueuedGhostMessage
        entry={entry({
          content: entityReferenceMarkdown(reference),
          metadata: { entity_references: [reference] },
        })}
        canEdit
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );

    const chip = screen.getByTestId("entity-reference-chip");
    expect(chip.getAttribute("href")).toBe("/t/task-1");
    expect(chip.getAttribute("target")).toBe("_self");
  });

  it("recomputes surviving references in edited document order", async () => {
    const task = taskReference();
    const issue = issueReference();
    const edited = `${entityReferenceMarkdown(issue)} then ${entityReferenceMarkdown(task)}`;
    const onSave = vi.fn(async () => {});

    renderWithProviders(
      <QueuedGhostMessage
        entry={entry({
          content: `${entityReferenceMarkdown(task)} then ${entityReferenceMarkdown(issue)}`,
          metadata: { entity_references: [task, issue] },
        })}
        canEdit
        onSave={onSave}
        onRemove={() => {}}
      />,
    );

    fireEvent.click(screen.getByTitle("Edit queued message"));
    fireEvent.change(screen.getByTestId("queue-edit-textarea"), { target: { value: edited } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledWith(edited, [issue, task]));
  });

  it("sends an explicit empty reference replacement after all links are removed", async () => {
    const reference = taskReference();
    const onSave = vi.fn(async () => {});

    renderWithProviders(
      <QueuedGhostMessage
        entry={entry({
          content: entityReferenceMarkdown(reference),
          metadata: { entity_references: [reference] },
        })}
        canEdit
        onSave={onSave}
        onRemove={() => {}}
      />,
    );

    fireEvent.click(screen.getByTitle("Edit queued message"));
    fireEvent.change(screen.getByTestId("queue-edit-textarea"), {
      target: { value: "reference removed" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledWith("reference removed", []));
  });
});
