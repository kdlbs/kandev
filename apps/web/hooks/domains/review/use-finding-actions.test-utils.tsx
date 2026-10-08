import { useLayoutEffect } from "react";
import { act, fireEvent, render, within } from "@testing-library/react";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { InlineReviewFinding } from "@/components/diff/inline-review-finding";
import { ReviewFindingsOverview } from "@/components/review/review-findings-overview";
import type { AppState } from "@/lib/state/store";
import type { TaskReviewFinding } from "@/lib/types/review";

const EMPTY_FINDINGS: TaskReviewFinding[] = [];

export function finding(overrides: Partial<TaskReviewFinding> = {}): TaskReviewFinding {
  return {
    id: "f-a",
    task_id: "task-a",
    run_id: "run-a",
    repository_id: "",
    repository_name: "",
    file_path: "parser.ts",
    start_line: 8,
    end_line: 9,
    side: "additions",
    severity: "blocker",
    category: "correctness",
    // i18n-exempt: Sample review finding data used only by the rendered test fixture.
    title: "Validate the incoming record",
    // i18n-exempt: Sample review finding data used only by the rendered test fixture.
    body: "The record needs validation before use.",
    suggestion: "",
    anchor_text: "",
    file_diff_hash: "fixture-hash",
    status: "open",
    created_at: "2026-10-08T07:00:00Z",
    updated_at: "2026-10-08T07:00:00Z",
    ...overrides,
  };
}

export function pendingRequest() {
  let resolve!: (response: { finding: TaskReviewFinding }) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<{ finding: TaskReviewFinding }>((accept, refuse) => {
    resolve = accept;
    reject = refuse;
  });
  return { promise, resolve, reject };
}

export async function acknowledge(
  request: ReturnType<typeof pendingRequest>,
  row: TaskReviewFinding,
) {
  await act(async () => request.resolve({ finding: row }));
}

export async function refuse(request: ReturnType<typeof pendingRequest>, reason: unknown) {
  await act(async () => request.reject(reason));
}

function FindingSurface({
  capture,
  copies,
}: {
  capture: (api: StoreApi<AppState>) => void;
  copies: number;
}) {
  const api = useAppStoreApi();
  useLayoutEffect(() => {
    capture(api);
  }, [api, capture]);
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const rows = useAppStore((state) =>
    taskId ? (state.taskReview.findingsByTaskId[taskId] ?? EMPTY_FINDINGS) : EMPTY_FINDINGS,
  );
  return (
    <>
      {Array.from({ length: copies }, (_, index) => (
        <section key={index} data-testid={`view-${index}`}>
          {rows.map((row) => (
            <div key={row.id} data-testid={row.id}>
              <InlineReviewFinding finding={row} />
            </div>
          ))}
          <ReviewFindingsOverview findings={rows} onNavigate={() => {}} />
        </section>
      ))}
    </>
  );
}

export function mountFindings(rows = [finding()], copies = 1) {
  let store: StoreApi<AppState> | undefined;
  const byTask: Record<string, TaskReviewFinding[]> = {};
  for (const row of rows) (byTask[row.task_id] ??= []).push(row);
  const mounted = render(
    <StateProvider>
      <ToastProvider>
        <FindingSurface
          copies={copies}
          capture={(api) => {
            store = api;
            api.getState().setActiveTask(rows[0].task_id);
            for (const [taskId, findings] of Object.entries(byTask)) {
              api.getState().setTaskReview(taskId, { runs: [], findings });
            }
          }}
        />
      </ToastProvider>
    </StateProvider>,
  );
  if (!store) throw new Error("Fixture did not capture the real store");
  return { store, container: mounted.container };
}

export function card(container: HTMLElement, id = "f-a", view = 0) {
  const surface = within(container).getByTestId(`view-${view}`);
  return within(within(surface).getByTestId(id));
}

export function click(
  container: HTMLElement,
  action: "resolve" | "dismiss" | "reopen",
  view = 0,
  id = "f-a",
) {
  fireEvent.click(card(container, id, view).getByTestId(`review-finding-${action}`));
}

export function status(container: HTMLElement, view = 0, id = "f-a") {
  return card(container, id, view)
    .getByTestId("review-finding-card")
    .getAttribute("data-finding-status");
}
