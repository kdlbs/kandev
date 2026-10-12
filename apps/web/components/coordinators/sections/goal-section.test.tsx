import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { Goal, GoalResponse } from "@/lib/api/domains/coordinator-api";

const putMock = vi.fn();
const toggleMock = vi.fn();
const metMock = vi.fn();
const getMock = vi.fn();
const toastError = vi.fn();
let contributor: {
  isDirty: boolean;
  canSave: boolean;
  save: () => Promise<void>;
  discard: () => void;
} | null = null;

vi.mock("@/lib/toast/sonner", () => ({ toast: { error: (...a: unknown[]) => toastError(...a) } }));
vi.mock("@/components/settings/settings-save-provider", () => ({
  useSettingsSaveContributor: (c: typeof contributor) => {
    contributor = c;
  },
}));
vi.mock("@/hooks/domains/coordinator/use-goal", async () => {
  const React = await import("react");
  return {
    useGoal: () => {
      const [state, setState] = React.useState<{ data: GoalResponse | null; status: string }>({
        data: null,
        status: "loading",
      });
      const reload = React.useCallback(() => {
        Promise.resolve(getMock()).then(
          (data) => setState({ data, status: "ready" }),
          () => setState((p) => (p.data ? p : { data: null, status: "error" })),
        );
      }, []);
      React.useEffect(() => reload(), [reload]);
      return { ...state, reload, retry: reload };
    },
  };
});
vi.mock("@/lib/api/domains/coordinator-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/coordinator-api")>();
  return {
    ...actual,
    putGoal: (...a: unknown[]) => putMock(...a),
    setGoalCriterionDone: (...a: unknown[]) => toggleMock(...a),
    markGoalMet: (...a: unknown[]) => metMock(...a),
  };
});

import { GoalSection } from "./goal-section";

const GOAL_NAME = "Ship the billing beta";
const MARK_MET = "goal-mark-met";

const MEASURES = {
  open_tasks: { current: 19, baseline: 23, direction: "down" as const },
  approved_7d: { current: 0, baseline: null, direction: "none_no_baseline" as const },
  rejected_7d: { current: 1, baseline: 1, direction: "none_small" as const },
};

function goal(overrides: Partial<Goal> = {}): Goal {
  return {
    id: "g1",
    coordinator_id: "c",
    name: GOAL_NAME,
    due_on: "2026-10-31",
    status: "active",
    criteria: [
      { id: "k1", text: "Docs", done: false },
      { id: "k2", text: "Tests", done: true },
    ],
    baseline: null,
    set_at: "2026-09-20T10:00:00Z",
    met_at: null,
    met_by: null,
    created_at: "2026-09-20T10:00:00Z",
    updated_at: "2026-09-20T10:00:00Z",
    ...overrides,
  } as Goal;
}

const response = (active: Goal | null): GoalResponse => ({
  active,
  last_met: null,
  measures: active ? MEASURES : null,
});

function renderSection(canManage = true) {
  render(<GoalSection workspaceId="ws" coordinatorId="c" canManage={canManage} />);
}

beforeEach(() => {
  [putMock, toggleMock, metMock, getMock, toastError].forEach((m) => m.mockReset());
  contributor = null;
});
afterEach(cleanup);

describe("GoalSection", () => {
  it("shows the stored goal with measures, direction, no direction and no baseline", async () => {
    getMock.mockResolvedValue(response(goal()));
    renderSection();
    expect(((await screen.findByTestId("goal-name")) as HTMLInputElement).value).toBe(GOAL_NAME);
    expect((screen.getByTestId("goal-due") as HTMLInputElement).value).toBe("2026-10-31");
    const measures = screen.getByTestId("goal-measures").textContent ?? "";
    expect(measures).toContain("23 to 19 down");
    expect(measures).toContain("No baseline");
    expect(measures).toContain("No direction yet");
  });

  it("gives a reader the values with no control, and 'No goal is set.' with no form", async () => {
    getMock.mockResolvedValue(response(goal()));
    renderSection(false);
    await screen.findByTestId("goal-name");
    expect(screen.queryByTestId(MARK_MET)).toBeNull();
    expect(screen.queryByTestId("goal-add-criterion")).toBeNull();
    expect((screen.getByTestId("goal-name") as HTMLInputElement).disabled).toBe(true);
    cleanup();
    getMock.mockResolvedValue(response(null));
    renderSection(false);
    expect(await screen.findByText("No goal is set.")).toBeTruthy();
    expect(screen.queryByTestId("goal-name")).toBeNull();
  });

  it("Set goal is disabled until the name is set, then PUTs without goal_id", async () => {
    getMock.mockResolvedValue(response(null));
    putMock.mockResolvedValue(goal());
    renderSection();
    const set = (await screen.findByTestId("goal-set")) as HTMLButtonElement;
    expect(set.disabled).toBe(true);
    expect(contributor?.isDirty).toBe(false);
    fireEvent.change(screen.getByTestId("goal-name"), { target: { value: "Ship" } });
    expect(set.disabled).toBe(false);
    getMock.mockResolvedValue(response(goal({ name: "Ship" })));
    fireEvent.click(set);
    await waitFor(() => expect(putMock).toHaveBeenCalled());
    expect(putMock.mock.calls[0][2]).toEqual({ name: "Ship", due_on: null, criteria: [] });
  });
});

describe("GoalSection saving", () => {
  it("keeps typed values and shows the 400 message under the field", async () => {
    getMock.mockResolvedValue(response(null));
    putMock.mockRejectedValue(
      new ApiError("bad", 400, { error: "name is too long", field: "name" }),
    );
    renderSection();
    fireEvent.change(await screen.findByTestId("goal-name"), { target: { value: "x" } });
    fireEvent.click(screen.getByTestId("goal-set"));
    expect(await screen.findByText("name is too long")).toBeTruthy();
    expect((screen.getByTestId("goal-name") as HTMLInputElement).value).toBe("x");
  });

  it("saves edits through the save bar with the goal id, due date cleared as null", async () => {
    getMock.mockResolvedValue(response(goal()));
    putMock.mockResolvedValue(goal({ name: "New", due_on: null }));
    renderSection();
    fireEvent.change(await screen.findByTestId("goal-name"), { target: { value: "New" } });
    fireEvent.change(screen.getByTestId("goal-due"), { target: { value: "" } });
    expect(contributor?.isDirty).toBe(true);
    await act(async () => contributor?.save());
    expect(putMock.mock.calls[0][2]).toEqual({
      goal_id: "g1",
      name: "New",
      due_on: null,
      criteria: [
        { id: "k1", text: "Docs" },
        { id: "k2", text: "Tests" },
      ],
    });
  });

  it("sends an empty name on an existing goal and shows the 400 under the field", async () => {
    getMock.mockResolvedValue(response(goal()));
    putMock.mockRejectedValue(
      new ApiError("bad", 400, { error: "name is required", field: "name" }),
    );
    renderSection();
    fireEvent.change(await screen.findByTestId("goal-name"), { target: { value: "" } });
    expect(contributor?.canSave).toBe(true);
    await act(async () => {
      await contributor?.save().catch(() => undefined);
    });
    expect(putMock).toHaveBeenCalledTimes(1);
    expect(await screen.findByText("name is required")).toBeTruthy();
  });

  it("reloads with a goal-changed message on 409 and on a stale criteria id 400", async () => {
    getMock.mockResolvedValue(response(goal()));
    renderSection();
    fireEvent.change(await screen.findByTestId("goal-name"), { target: { value: "Edit" } });
    putMock.mockRejectedValueOnce(new ApiError("conflict", 409, {}));
    getMock.mockResolvedValue(response(goal({ name: "Server" })));
    await act(async () => {
      await contributor?.save().catch(() => undefined);
    });
    expect(await screen.findByText(/This goal changed while you were editing/)).toBeTruthy();
    await waitFor(() =>
      expect((screen.getByTestId("goal-name") as HTMLInputElement).value).toBe("Server"),
    );
    fireEvent.change(screen.getByTestId("goal-name"), { target: { value: "Edit2" } });
    putMock.mockRejectedValueOnce(
      new ApiError("bad", 400, { error: "unknown id", field: "criteria[0].id" }),
    );
    getMock.mockResolvedValue(response(goal({ name: "Server2" })));
    await act(async () => {
      await contributor?.save().catch(() => undefined);
    });
    await waitFor(() =>
      expect((screen.getByTestId("goal-name") as HTMLInputElement).value).toBe("Server2"),
    );
  });
});

const STATE_ATTR = "data-state";
const CHECKED = "checked";
const CRITERION_1 = "Exit criterion 1 done";
const CRITERION_2 = "Exit criterion 2 done";

describe("GoalSection toggles and completion", () => {
  it("toggles a saved criterion immediately, queues a second click and lets the last response win", async () => {
    getMock.mockResolvedValue(response(goal()));
    const resolvers: ((g: Goal) => void)[] = [];
    toggleMock.mockImplementation(() => new Promise<Goal>((r) => resolvers.push(r)));
    renderSection();
    await screen.findByTestId("goal-name");
    const box = screen.getByLabelText(CRITERION_1);
    fireEvent.click(box);
    fireEvent.click(box);
    fireEvent.click(box);
    expect(toggleMock).toHaveBeenCalledTimes(1);
    expect(toggleMock.mock.calls[0].slice(2)).toEqual(["k1", true]);
    await act(async () => resolvers[0](goal()));
    await waitFor(() => expect(toggleMock).toHaveBeenCalledTimes(2));
    expect(toggleMock.mock.calls[1].slice(2)).toEqual(["k1", true]);
    await act(async () =>
      resolvers[1](
        goal({ criteria: [{ id: "k1", text: "Docs", done: true }, goal().criteria[1]] }),
      ),
    );
    expect(box.getAttribute(STATE_ATTR)).toBe(CHECKED);
    expect(contributor?.isDirty).toBe(false);
  });

  it("keeps a criterion's optimistic state when another criterion's stale response arrives last", async () => {
    getMock.mockResolvedValue(
      response(
        goal({
          criteria: [
            { id: "k1", text: "Docs", done: false },
            { id: "k2", text: "Tests", done: false },
          ],
        }),
      ),
    );
    const resolvers: Record<string, (g: Goal) => void> = {};
    toggleMock.mockImplementation(
      (...a: unknown[]) => new Promise<Goal>((r) => (resolvers[a[2] as string] = r)),
    );
    renderSection();
    await screen.findByTestId("goal-name");
    const one = screen.getByLabelText(CRITERION_1);
    const two = screen.getByLabelText(CRITERION_2);
    fireEvent.click(one);
    fireEvent.click(two);
    const both = goal({
      criteria: [
        { id: "k1", text: "Docs", done: true },
        { id: "k2", text: "Tests", done: true },
      ],
    });
    const onlyOne = goal({
      criteria: [
        { id: "k1", text: "Docs", done: true },
        { id: "k2", text: "Tests", done: false },
      ],
    });
    await act(async () => resolvers.k2(both));
    await act(async () => resolvers.k1(onlyOne));
    expect(one.getAttribute(STATE_ATTR)).toBe(CHECKED);
    expect(two.getAttribute(STATE_ATTR)).toBe(CHECKED);
  });
});

describe("GoalSection toggle failures", () => {
  it("toasts and reloads when a toggle fails, then accepts another click", async () => {
    getMock.mockImplementation(() => Promise.resolve(response(goal())));
    toggleMock.mockRejectedValueOnce(new ApiError("e", 500, {}));
    renderSection();
    await screen.findByTestId("goal-name");
    const box = screen.getByLabelText(CRITERION_1);
    fireEvent.click(box);
    await waitFor(() =>
      expect(toastError).toHaveBeenCalledWith("Could not save the goal. Try again."),
    );
    await waitFor(() => expect(getMock).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(box.getAttribute(STATE_ATTR)).toBe("unchecked"));
    toggleMock.mockResolvedValue(goal());
    fireEvent.click(box);
    await waitFor(() => expect(toggleMock).toHaveBeenCalledTimes(2));
  });

  it("stays silent when a toggle is answered 404 and reloads", async () => {
    getMock.mockResolvedValue(response(goal()));
    toggleMock.mockRejectedValueOnce(new ApiError("nf", 404, {}));
    renderSection();
    await screen.findByTestId("goal-name");
    fireEvent.click(screen.getByLabelText(CRITERION_1));
    await waitFor(() => expect(getMock).toHaveBeenCalledTimes(2));
    expect(toastError).not.toHaveBeenCalled();
  });

  it("disables the checkbox of an unsaved criterion", async () => {
    getMock.mockResolvedValue(response(goal()));
    renderSection();
    await screen.findByTestId("goal-name");
    fireEvent.click(screen.getByTestId("goal-add-criterion"));
    const box = screen.getByLabelText("Exit criterion 3 done") as HTMLButtonElement;
    expect(box.disabled).toBe(true);
  });

  it("confirms Mark milestone met naming the goal, mentions discard when dirty, sends goal_id once", async () => {
    getMock.mockResolvedValue(response(goal()));
    let resolve: (g: Goal) => void = () => {};
    metMock.mockReturnValue(new Promise<Goal>((r) => (resolve = r)));
    renderSection();
    fireEvent.change(await screen.findByTestId("goal-name"), {
      target: { value: GOAL_NAME },
    });
    fireEvent.change(screen.getByTestId("goal-due"), { target: { value: "2026-11-01" } });
    fireEvent.click(screen.getByTestId(MARK_MET));
    const dialog = await screen.findByTestId("goal-mark-met-dialog");
    expect(dialog.textContent).toContain(GOAL_NAME);
    expect(dialog.textContent).toContain("Unsaved changes to this goal will be discarded.");
    const confirm = screen.getByTestId("goal-mark-met-confirm") as HTMLButtonElement;
    fireEvent.click(confirm);
    expect(confirm.disabled).toBe(true);
    fireEvent.click(confirm);
    expect(metMock).toHaveBeenCalledTimes(1);
    expect(metMock).toHaveBeenCalledWith("ws", "c", "g1");
    getMock.mockResolvedValue(response(null));
    await act(async () => resolve(goal({ status: "met" })));
    expect(await screen.findByTestId("goal-set")).toBeTruthy();
  });

  it("shows a permission error on 403 and a generic one on 500 without keeping a change", async () => {
    getMock.mockResolvedValue(response(goal()));
    renderSection();
    await screen.findByTestId("goal-name");
    metMock.mockRejectedValueOnce(new ApiError("f", 403, {}));
    fireEvent.click(screen.getByTestId(MARK_MET));
    fireEvent.click(await screen.findByTestId("goal-mark-met-confirm"));
    expect(await screen.findByText("You do not have permission to change this goal.")).toBeTruthy();
    fireEvent.click(screen.getByTestId(MARK_MET));
    metMock.mockRejectedValueOnce(new ApiError("e", 500, {}));
    fireEvent.click(await screen.findByTestId("goal-mark-met-confirm"));
    expect(await screen.findByText("Could not save the goal. Try again.")).toBeTruthy();
    expect((screen.getByTestId("goal-name") as HTMLInputElement).value).toBe(GOAL_NAME);
  });

  it("shows the load error with retry", async () => {
    getMock.mockRejectedValueOnce(new Error("x"));
    renderSection();
    expect(await screen.findByTestId("goal-error")).toBeTruthy();
    getMock.mockResolvedValue(response(null));
    fireEvent.click(screen.getByText("Retry"));
    expect(await screen.findByTestId("goal-set")).toBeTruthy();
  });
});
