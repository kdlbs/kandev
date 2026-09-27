import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { AttentionTask, QueueItem } from "@/lib/coordinator/attention";

vi.mock("@/components/state-provider", () => ({
  useAppStore: () => null,
}));

import { QueueGroup } from "./queue-group";

afterEach(cleanup);

function task(id: string): AttentionTask {
  return { id, title: `Task ${id}`, identifier: `KAN-${id}` };
}

function item(id: string, group: QueueItem["group"] = "working"): QueueItem {
  return { group, id, task: task(id), lastActivityAtMs: undefined, ageMs: 60_000 };
}

describe("QueueGroup", () => {
  it("shows Working expanded with its count and rows", () => {
    render(
      <QueueGroup group="working" items={[item("1"), item("2")]} stepNameByTaskId={new Map()} />,
    );
    expect(screen.getByText("Working")).not.toBeNull();
    expect(screen.getByText("2")).not.toBeNull();
    expect(screen.getByText("KAN-1")).not.toBeNull();
    expect(screen.getByText("KAN-2")).not.toBeNull();
  });

  it("shows Done collapsed by default, expandable via its trigger", async () => {
    render(<QueueGroup group="done" items={[item("3", "done")]} stepNameByTaskId={new Map()} />);
    const trigger = screen.getByRole("button", { name: /Done/ });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    expect(screen.queryByText("KAN-3")).toBeNull();

    fireEvent.click(trigger);
    expect(await screen.findByText("KAN-3")).not.toBeNull();
  });

  it("shows Other collapsed by default", () => {
    render(<QueueGroup group="other" items={[item("4", "other")]} stepNameByTaskId={new Map()} />);
    expect(screen.getByText("Other")).not.toBeNull();
    expect(screen.queryByText("KAN-4")).toBeNull();
  });
});
