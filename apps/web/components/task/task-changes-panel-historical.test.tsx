import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { SelectedDiff } from "@/hooks/use-session-layout-state";
import { TaskChangesPanel } from "./task-changes-panel";
import { TaskArchivedProvider } from "./task-archived-context";

vi.mock("./historical-turn-diff-viewer", () => ({
  HistoricalTurnDiffViewer: ({
    target,
    detailActive,
  }: {
    target: { changeSetId: string };
    detailActive?: boolean;
  }) => (
    <div
      data-testid="retained-historical-diff"
      data-change-set-id={target.changeSetId}
      data-detail-active={String(detailActive)}
    />
  ),
}));

afterEach(cleanup);

describe("TaskChangesPanel historical routing", () => {
  it.each([true, false])("forwards detail demand %s to retained history", (detailActive) => {
    render(
      <TaskChangesPanel
        detailActive={detailActive}
        selectedDiff={{
          path: "file.ts",
          historical: { sessionId: "session", changeSetId: "capture" },
        }}
        onClearSelected={() => {}}
      />,
    );
    expect(screen.getByTestId("retained-historical-diff").getAttribute("data-detail-active")).toBe(
      String(detailActive),
    );
  });

  it("opens retained history for an archived task before live workspace guards", () => {
    const selectedDiff: SelectedDiff = {
      path: "src/retained.ts",
      historical: { sessionId: "session-1", changeSetId: "turn-change-older" },
    };
    render(
      <TaskArchivedProvider value={{ isArchived: true }}>
        <TaskChangesPanel selectedDiff={selectedDiff} onClearSelected={() => {}} />
      </TaskArchivedProvider>,
    );

    expect(screen.getByTestId("retained-historical-diff").getAttribute("data-change-set-id")).toBe(
      "turn-change-older",
    );
    expect(screen.queryByText("task:workspaceUnavailableArchived")).toBeNull();
  });
});
