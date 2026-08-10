import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueuedGhostMessage, canMergeEntry, canMergeWithAbove } from "./queued-ghost-message";
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
const MERGE_TESTID = "queue-entry-merge";
const EDIT_TESTID = "queue-entry-edit";
const REMOVE_TESTID = "queue-entry-remove";
const EDIT_TITLE = "Edit queued message";
const MERGE_TITLE = "Merge with above";

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

describe("QueuedGhostMessage reorder handle", () => {
  const HANDLE_TESTID = "queue-grab-handle";

  function renderRow(
    overrides: Partial<QueuedMessage> = {},
    props: Partial<React.ComponentProps<typeof QueuedGhostMessage>> = {},
  ) {
    return render(
      <QueuedGhostMessage
        entry={entry(overrides)}
        canEdit={false}
        onSave={vi.fn()}
        onRemove={vi.fn()}
        {...props}
      />,
    );
  }

  it("renders a dotted grab handle with localized labels", () => {
    renderRow();
    const handle = screen.getByTestId(HANDLE_TESTID);
    expect(handle.getAttribute("aria-label")).toBe("Reorder queued message");
    expect(handle.getAttribute("aria-roledescription")).toBe("sortable");
    // 2x3 dot grid reads as a dotted grip.
    expect(handle.querySelectorAll('[aria-hidden="true"] span').length).toBe(6);
  });

  it("shows the handle without hovering on a fine pointer", () => {
    renderRow();
    const handle = screen.getByTestId(HANDLE_TESTID);
    // A mounted handle is an always-visible drag affordance on every pointer.
    expect(handle.className).toContain("opacity-100");
    expect(handle.className).not.toContain("opacity-0");
    expect(handle.className).not.toContain("group-hover:opacity-100");
  });

  it("keeps the fine-pointer grip small and unobtrusive", () => {
    renderRow();
    const handle = screen.getByTestId(HANDLE_TESTID);
    expect(handle.className).toContain("left-1");
    expect(handle.className).toContain("h-3 w-3");
    expect(handle.className).toContain("border-0");
    expect(handle.className).toContain("bg-transparent");
    expect(handle.className).toContain("shadow-none");
    const grip = handle.querySelector('[aria-hidden="true"]')!;
    expect(grip.className).toContain("gap-0.5");
    const dot = handle.querySelector('[aria-hidden="true"] span')!;
    expect(dot.className).toContain("h-px w-px");
  });

  it("attaches the coarse-pointer handle to the box edge in the background", () => {
    renderRow();
    const handle = screen.getByTestId(HANDLE_TESTID);
    // Coarse pointers: flush at the left edge, behind the content (z-0, no
    // chip surface) so the dots read as part of the box edge.
    expect(handle.className).toContain("[@media(pointer:coarse)]:left-0");
    expect(handle.className).toContain("[@media(pointer:coarse)]:z-0");
    expect(handle.className).toContain("[@media(pointer:coarse)]:bg-transparent");
    expect(handle.className).toContain("[@media(pointer:coarse)]:shadow-none");
    expect(handle.className).toContain("[@media(pointer:coarse)]:h-11");
    expect(handle.className).toContain("[@media(pointer:coarse)]:w-11");
  });

  it("keeps the position label painted above the handle and pointer-transparent", () => {
    renderRow();
    // The label/icon cluster sits above the background handle (relative) and
    // passes touches through to it (pointer-events-none).
    const label = screen.getByLabelText("Position #1");
    const cluster = label.parentElement!;
    expect(cluster.className).toContain("relative");
    expect(cluster.className).toContain("pointer-events-none");
  });

  it("centers the position, sender icon, and message content", () => {
    renderRow();
    const row = screen.getByTestId("queue-entry");
    expect(row.className).toContain("items-center");
    expect(row.className).not.toContain("items-start");
    const label = screen.getByLabelText("Position #1");
    expect(label.parentElement!.className).not.toContain("mt-0.5");
  });

  it("disables the handle while reordering is unavailable", () => {
    renderRow({}, { canDrag: false });
    const handle = screen.getByTestId(HANDLE_TESTID) as HTMLButtonElement;
    expect(handle.disabled).toBe(true);
    expect(handle.className).toContain("cursor-not-allowed");
  });

  it("hides the handle while the row is being edited", () => {
    renderRow({ queued_by: "user-1" }, { canEdit: true });
    fireEvent.click(screen.getByTestId(EDIT_TESTID));
    expect(screen.queryByTestId(HANDLE_TESTID)).toBeNull();
  });
});

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
    fireEvent.click(screen.getByTitle(EDIT_TITLE));
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

    fireEvent.click(screen.getByTitle(EDIT_TITLE));
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

    fireEvent.click(screen.getByTitle(EDIT_TITLE));
    fireEvent.change(screen.getByTestId("queue-edit-textarea"), {
      target: { value: "reference removed" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(onSave).toHaveBeenCalledWith("reference removed", []));
  });
});

describe("QueuedGhostMessage merge control", () => {
  it("hides the merge button for the first entry (no entry above)", () => {
    render(
      <QueuedGhostMessage entry={entry()} canEdit onSave={async () => {}} onRemove={() => {}} />,
    );
    expect(screen.queryByTestId(MERGE_TESTID)).toBeNull();
  });

  it("shows the merge button when canMerge is set", () => {
    render(
      <QueuedGhostMessage
        entry={entry()}
        canEdit
        canMerge
        onMerge={vi.fn()}
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );
    const button = screen.getByTestId(MERGE_TESTID);
    expect(button.getAttribute("title")).toBe(MERGE_TITLE);
  });

  it("clicking the merge button invokes onMerge", () => {
    const onMerge = vi.fn();
    render(
      <QueuedGhostMessage
        entry={entry()}
        canEdit
        canMerge
        onMerge={onMerge}
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );
    fireEvent.click(screen.getByTestId(MERGE_TESTID));
    expect(onMerge).toHaveBeenCalledTimes(1);
  });

  it("hides the merge button for workflow entries", () => {
    render(
      <QueuedGhostMessage
        entry={entry({
          queued_by: "workflow",
          metadata: { workflow_message: true },
        })}
        canEdit={false}
        canMerge
        onMerge={vi.fn()}
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );
    expect(screen.queryByTestId(MERGE_TESTID)).toBeNull();
  });

  it("hides the merge button while editing", () => {
    render(
      <QueuedGhostMessage
        entry={entry()}
        canEdit
        canMerge
        onMerge={vi.fn()}
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );
    fireEvent.click(screen.getByTitle(EDIT_TITLE));
    expect(screen.queryByTestId(MERGE_TESTID)).toBeNull();
  });
});

describe("QueuedGhostMessage row actions", () => {
  it("shows Remove independently when an entry is not editable", () => {
    const onRemove = vi.fn();
    render(
      <QueuedGhostMessage
        entry={entry({ queued_by: "agent" })}
        canEdit={false}
        canRemove
        onSave={async () => {}}
        onRemove={onRemove}
      />,
    );

    expect(screen.queryByTestId(EDIT_TESTID)).toBeNull();
    fireEvent.click(screen.getByTestId(REMOVE_TESTID));
    expect(onRemove).toHaveBeenCalledTimes(1);
  });

  it("keeps Edit independent from Remove", () => {
    render(
      <QueuedGhostMessage
        entry={entry()}
        canEdit
        canRemove={false}
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );

    expect(screen.getByTestId(EDIT_TESTID)).toBeTruthy();
    expect(screen.queryByTestId(REMOVE_TESTID)).toBeNull();
  });

  it("keeps compact desktop controls but provides coarse-pointer visibility and 44px targets", () => {
    render(
      <QueuedGhostMessage
        entry={entry({ queued_by: "workflow" })}
        canEdit={false}
        canRemove
        onSave={async () => {}}
        onRemove={() => {}}
      />,
    );

    const remove = screen.getByTestId(REMOVE_TESTID);
    expect(remove.className).toContain("h-6");
    expect(remove.className).toContain("w-6");
    expect(remove.className).toContain("[@media(pointer:coarse)]:h-11");
    expect(remove.className).toContain("[@media(pointer:coarse)]:w-11");
    expect(remove.parentElement?.className).toContain("[@media(pointer:coarse)]:opacity-100");
  });
});

describe("canMergeEntry / canMergeWithAbove gating", () => {
  it("treats user entries as mergeable", () => {
    expect(canMergeEntry(entry({ queued_by: "user-1" }))).toBe(true);
  });

  it("treats agent entries as mergeable", () => {
    expect(canMergeEntry(entry({ queued_by: "agent" }))).toBe(true);
  });

  it("treats workflow and system entries as non-mergeable", () => {
    expect(canMergeEntry(entry({ queued_by: "workflow" }))).toBe(false);
    expect(canMergeEntry(entry({ queued_by: "" }))).toBe(false);
  });

  it("treats server entries as non-mergeable", () => {
    expect(canMergeEntry(entry({ queued_by: "server" }))).toBe(false);
  });

  it("allows user behind user owned by the caller", () => {
    const above = entry({ id: "q-a", queued_by: "user-1" });
    const below = entry({ id: "q-b", queued_by: "user-1" });
    expect(canMergeWithAbove(below, above, "user-1")).toBe(true);
  });

  it("allows user rows under the default user caller identity", () => {
    const above = entry({ id: "q-a", queued_by: "user" });
    const below = entry({ id: "q-b", queued_by: "user" });
    expect(canMergeWithAbove(below, above)).toBe(true);
  });

  it("rejects user behind user owned by a different owner", () => {
    const above = entry({ id: "q-a", queued_by: "user-1" });
    const below = entry({ id: "q-b", queued_by: "user-2" });
    expect(canMergeWithAbove(below, above, "user-1")).toBe(false);
    expect(canMergeWithAbove(below, above, "user-2")).toBe(false);
  });

  it("rejects user rows the caller does not own", () => {
    const above = entry({ id: "q-a", queued_by: "user-1" });
    const below = entry({ id: "q-b", queued_by: "user-1" });
    expect(canMergeWithAbove(below, above, "user-2")).toBe(false);
  });

  it("rejects user behind a server row", () => {
    const above = entry({ id: "q-a", queued_by: "server" });
    const below = entry({ id: "q-b", queued_by: "user-1" });
    expect(canMergeWithAbove(below, above, "user-1")).toBe(false);
  });

  it("allows agent behind agent from the same sender task", () => {
    const above = entry({ id: "q-a", queued_by: "agent", metadata: { sender_task_id: "task-7" } });
    const below = entry({ id: "q-b", queued_by: "agent", metadata: { sender_task_id: "task-7" } });
    expect(canMergeWithAbove(below, above)).toBe(true);
  });

  it("rejects agent behind agent from different sender tasks", () => {
    const above = entry({ id: "q-a", queued_by: "agent", metadata: { sender_task_id: "task-7" } });
    const below = entry({ id: "q-b", queued_by: "agent", metadata: { sender_task_id: "task-8" } });
    expect(canMergeWithAbove(below, above)).toBe(false);
  });

  it("rejects agent behind agent when sender_task_id is missing", () => {
    const above = entry({ id: "q-a", queued_by: "agent" });
    const below = entry({ id: "q-b", queued_by: "agent" });
    expect(canMergeWithAbove(below, above)).toBe(false);
  });

  it("rejects user behind agent and agent behind user", () => {
    const agent = entry({ id: "q-a", queued_by: "agent", metadata: { sender_task_id: "task-7" } });
    const user = entry({ id: "q-b", queued_by: "user-1" });
    expect(canMergeWithAbove(user, agent)).toBe(false);
    expect(canMergeWithAbove(agent, user)).toBe(false);
  });

  it("rejects when no entry is above", () => {
    expect(canMergeWithAbove(entry(), undefined)).toBe(false);
  });

  it("rejects a workflow entry behind a user entry", () => {
    const above = entry({ id: "q-a", queued_by: "user-1" });
    const below = entry({ id: "q-b", queued_by: "workflow" });
    expect(canMergeWithAbove(below, above)).toBe(false);
  });

  it("rejects a server entry behind a user entry", () => {
    const above = entry({ id: "q-a", queued_by: "user-1" });
    const below = entry({ id: "q-b", queued_by: "server" });
    expect(canMergeWithAbove(below, above)).toBe(false);
  });
});
