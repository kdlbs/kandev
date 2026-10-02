import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { AttentionTask, QueueItem } from "@/lib/coordinator/attention";
import type { TaskPR } from "@/lib/types/github";
import { QueueGroup } from "./queue-group";

afterEach(cleanup);

function task(id: string): AttentionTask {
  return { id, title: `Task ${id}`, identifier: `KAN-${id}` };
}

function item(id: string, group: QueueItem["group"] = "working"): QueueItem {
  return { group, id, task: task(id), lastActivityAtMs: undefined, ageMs: 60_000 };
}

const NO_PRS: ReadonlyMap<string, TaskPR[]> = new Map();

describe("QueueGroup", () => {
  it("shows Working expanded with its count and rows", () => {
    render(
      <QueueGroup
        group="working"
        items={[item("1"), item("2")]}
        stepNameByTaskId={new Map()}
        prsByTaskId={NO_PRS}
      />,
    );
    expect(screen.getByText("Working")).not.toBeNull();
    expect(screen.getByText("2")).not.toBeNull();
    expect(screen.getByText("KAN-1")).not.toBeNull();
    expect(screen.getByText("KAN-2")).not.toBeNull();
  });

  it("shows Done collapsed by default, expandable via its trigger", async () => {
    render(
      <QueueGroup
        group="done"
        items={[item("3", "done")]}
        stepNameByTaskId={new Map()}
        prsByTaskId={NO_PRS}
      />,
    );
    const trigger = screen.getByRole("button", { name: /Done/ });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText("KAN-3")).toBeNull();

    fireEvent.click(trigger);
    expect(await screen.findByText("KAN-3")).not.toBeNull();
  });

  it("shows Other collapsed by default", () => {
    render(
      <QueueGroup
        group="other"
        items={[item("4", "other")]}
        stepNameByTaskId={new Map()}
        prsByTaskId={NO_PRS}
      />,
    );
    expect(screen.getByText("Other")).not.toBeNull();
    expect(screen.queryByText("KAN-4")).toBeNull();
  });
});

describe("QueueGroup - section styling", () => {
  it("heads a group with an uppercase eyebrow, not a title competing with the rows", () => {
    render(
      <QueueGroup
        group="working"
        items={[item("1")]}
        stepNameByTaskId={new Map()}
        prsByTaskId={NO_PRS}
      />,
    );
    const heading = screen.getByRole("heading", { level: 3 });
    expect(heading.className).toContain("uppercase");
    expect(heading.className).toContain("text-muted-foreground");
  });

  it("puts the rows in one bordered panel", () => {
    render(
      <QueueGroup
        group="working"
        items={[item("1")]}
        stepNameByTaskId={new Map()}
        prsByTaskId={NO_PRS}
      />,
    );
    const rows = screen.getByTestId("queue-group-rows");
    expect(rows.className).toContain("border");
    expect(rows.className).toContain("bg-card");
  });
});

describe("QueueGroup - Ready to merge with the phase-2 flag", () => {
  const withPr: ReadonlyMap<string, TaskPR[]> = new Map([
    [
      "7",
      [{ id: "p", task_id: "7", state: "open", pr_url: "https://example.test/pr/7" } as TaskPR],
    ],
  ]);
  const props = {
    group: "ready_to_merge" as const,
    items: [item("7", "ready_to_merge")],
    stepNameByTaskId: new Map<string, string>(),
    prsByTaskId: withPr,
  };

  it("flag off: a seeded pr_url adds no Open the PR link and no header line", () => {
    render(<QueueGroup {...props} canManage />);
    expect(screen.getByText("KAN-7")).not.toBeNull();
    expect(screen.queryByRole("link", { name: "Open the PR" })).toBeNull();
    expect(screen.queryByText("Merging a pull request is always human.")).toBeNull();
  });

  it("flag on: the same row shows Open the PR and the header line", () => {
    render(<QueueGroup {...props} phase2 canManage />);
    expect(screen.getByRole("link", { name: "Open the PR" })).not.toBeNull();
    expect(screen.getByText("Merging a pull request is always human.")).not.toBeNull();
  });
});
