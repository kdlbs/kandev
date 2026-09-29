import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";

const mockReplace = vi.fn();
const mockSetup = vi.fn();
const mockAdd = vi.fn();

vi.mock("@/lib/routing/client-router", () => ({
  useRouter: () => ({ push: vi.fn(), replace: mockReplace }),
}));
vi.mock("@/hooks/domains/settings/use-settings-data", () => ({ useSettingsData: vi.fn() }));
vi.mock("@/hooks/domains/coordinator/use-workspace-boards", () => ({
  useWorkspaceBoards: () => ({
    boards: [
      { id: "b1", name: "Product", hidden: false },
      { id: "b2", name: "Release", hidden: false },
    ],
    status: "ready",
    retry: vi.fn(),
  }),
}));
vi.mock("@/lib/api/domains/coordinator-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/coordinator-api")>()),
  setupCoordinator: (...args: unknown[]) => mockSetup(...args),
}));

const storeState = {
  addCoordinator: (...args: unknown[]) => mockAdd(...args),
  agentProfiles: {
    items: [
      { id: "a1", label: "Claude", agent_id: "c", agent_name: "Claude", cli_passthrough: false },
    ],
  },
  executors: {
    items: [
      {
        id: "x1",
        name: "Local",
        type: "local",
        status: "ready",
        is_system: false,
        profiles: [{ id: "p1", executor_id: "x1", name: "worktree" }],
      },
    ],
  },
  workspaces: {
    items: [{ id: "w1", default_agent_profile_id: "a1", default_executor_id: "x1" }],
  },
};
vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof storeState) => unknown) => selector(storeState),
}));

import { CoordinatorSetup } from "./coordinator-setup";

function open() {
  render(
    <TooltipProvider>
      <CoordinatorSetup workspaceId="w1" />
    </TooltipProvider>,
  );
}

const PLANNER = "Planner";
const NEXT_ID = "setup-next";
const BACK_ID = "setup-back";
const SKIP_ID = "setup-skip";
const FINISH_ID = "setup-finish";

const click = (id: string) => fireEvent.click(screen.getByTestId(id));
const next = () => click(NEXT_ID);

function typeName(value: string) {
  fireEvent.change(screen.getByLabelText("Name"), { target: { value } });
}

function toReview({ skipGoal = true } = {}) {
  typeName(PLANNER);
  next();
  next();
  if (skipGoal) click(SKIP_ID);
  else next();
  click(SKIP_ID);
  next();
}

beforeEach(() => {
  mockSetup.mockResolvedValue({ id: "new-1" });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("CoordinatorSetup navigation", () => {
  it("starts on step 1 with Next only, disabled until the name is set", () => {
    open();
    expect(screen.getByTestId("setup-step-identity").getAttribute("aria-current")).toBe("step");
    expect(screen.queryByTestId(BACK_ID)).toBeNull();
    expect((screen.getByTestId(NEXT_ID) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByTestId("coordinator-name-error")).toBeNull();
    typeName(PLANNER);
    expect((screen.getByTestId(NEXT_ID) as HTMLButtonElement).disabled).toBe(false);
  });

  it("shows the phone step line and marks a step left while valid", () => {
    open();
    expect(screen.getByTestId("setup-step-compact").textContent).toContain(
      "Step 1 of 6: Who runs it",
    );
    typeName(PLANNER);
    next();
    expect(screen.getByTestId("setup-step-done-identity")).toBeDefined();
    expect(screen.getByTestId("setup-step-watches").getAttribute("aria-current")).toBe("step");
  });

  it("offers Skip only on the goal and context steps and Back keeps values", () => {
    open();
    typeName(PLANNER);
    next();
    expect(screen.queryByTestId(SKIP_ID)).toBeNull();
    next();
    expect(screen.getByTestId(SKIP_ID)).toBeDefined();
    click(BACK_ID);
    click(BACK_ID);
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe(PLANNER);
  });

  it("Skip clears the goal values", () => {
    open();
    typeName(PLANNER);
    next();
    next();
    fireEvent.change(screen.getByTestId("goal-name"), { target: { value: "Ship" } });
    click(SKIP_ID);
    click(BACK_ID);
    expect((screen.getByTestId("goal-name") as HTMLInputElement).value).toBe("");
  });

  it("requires a valid goal once any goal field has a value", () => {
    open();
    typeName(PLANNER);
    next();
    next();
    fireEvent.click(screen.getByTestId("goal-add-criterion"));
    expect((screen.getByTestId(NEXT_ID) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("Enter a milestone of 1 to 120 characters.")).toBeDefined();
    fireEvent.change(screen.getByTestId("goal-name"), { target: { value: "Ship" } });
    expect((screen.getByTestId(NEXT_ID) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByText("Enter 1 to 200 characters or remove this criterion.")).toBeDefined();
    fireEvent.change(screen.getByLabelText("Exit criterion 1"), { target: { value: "Done" } });
    expect((screen.getByTestId(NEXT_ID) as HTMLButtonElement).disabled).toBe(false);
  });
});

describe("CoordinatorSetup review", () => {
  it("lists 12 rows in order with the owner and a Change button per row", () => {
    open();
    toReview();
    const rows = screen.getAllByTestId(/^setup-review-row-/);
    expect(rows.map((r) => r.getAttribute("data-testid"))).toEqual(
      [
        "name",
        "agent",
        "executor",
        "watches",
        "goal",
        "context",
        "create_task",
        "start_agent",
        "message",
        "move",
        "resume",
        "stop",
      ].map((id) => `setup-review-row-${id}`),
    );
    expect(screen.getByTestId("setup-review-value-name").textContent).toContain("Planner");
    expect(screen.getByTestId("setup-review-value-agent").textContent).toContain("Claude");
    expect(screen.getByTestId("setup-review-value-executor").textContent).toContain("worktree");
    expect(screen.getByTestId("setup-review-value-watches").textContent).toContain("Every board");
    expect(screen.getByTestId("setup-review-value-goal").textContent).toContain("Not set");
    expect(screen.getByTestId("setup-review-value-context").textContent).toContain("Not set");
    expect(screen.getByTestId("setup-review-value-stop").textContent).toContain("Denied");
    expect(screen.getByTestId("setup-review-value-create_task").textContent).toContain(
      "Requires approval",
    );
  });

  it("names the chosen boards in workspace order and formats the goal", () => {
    open();
    typeName(PLANNER);
    next();
    fireEvent.click(screen.getByRole("switch"));
    next();
    fireEvent.change(screen.getByTestId("goal-name"), { target: { value: "Ship" } });
    fireEvent.click(screen.getByTestId("goal-add-criterion"));
    fireEvent.change(screen.getByLabelText("Exit criterion 1"), { target: { value: "One" } });
    next();
    fireEvent.change(screen.getByLabelText("Context"), { target: { value: "  \n " } });
    next();
    next();
    expect(screen.getByTestId("setup-review-value-watches").textContent).toContain(
      "Product, Release",
    );
    expect(screen.getByTestId("setup-review-value-goal").textContent).toContain(
      "Ship, no due date, 1 criterion",
    );
    expect(screen.getByTestId("setup-review-value-context").textContent).toContain("Not set");
  });

  it("Change goes to the step with values kept and Next returns to Review", () => {
    open();
    toReview();
    click("setup-review-change-name");
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe(PLANNER);
    next();
    expect(screen.getByTestId("setup-review")).toBeDefined();
  });

  it("Back from a Change step ends the shortcut", () => {
    open();
    toReview();
    click("setup-review-change-context");
    click(BACK_ID);
    next();
    expect(screen.queryByTestId("setup-review")).toBeNull();
  });
});

describe("CoordinatorSetup finish", () => {
  it("sends the whole payload once and opens the Configure page", async () => {
    open();
    toReview();
    click(FINISH_ID);
    click(FINISH_ID);
    await waitFor(() =>
      expect(mockReplace).toHaveBeenCalledWith("/settings/workspaces/w1/coordinators/new-1"),
    );
    expect(mockSetup).toHaveBeenCalledTimes(1);
    expect(mockSetup).toHaveBeenCalledWith("w1", {
      name: PLANNER,
      agent_profile_id: "a1",
      executor_profile_id: "p1",
      context: "",
      watches: { scope: "all" },
      policy: {
        actions: {
          create_task: "requires_approval",
          start_agent: "denied",
          message: "requires_approval",
          move: "requires_approval",
          resume: "requires_approval",
          stop: "denied",
        },
      },
    });
    expect(mockAdd).toHaveBeenCalledWith({ id: "new-1" });
  });

  it("returns to the step a 400 names, keeps values and holds Next until the field is edited", async () => {
    mockSetup.mockRejectedValue(
      new ApiError("bad", 400, {
        error: "server english",
        step: "watches",
        field: "watches",
        code: "watches_foreign_workflow",
      }),
    );
    open();
    toReview();
    click(FINISH_ID);
    await waitFor(() => expect(screen.getByTestId("setup-body-watches")).toBeDefined());
    expect(screen.getByTestId("watches-error").textContent).toMatch(
      /does not belong to this workspace/,
    );
    expect(screen.queryByText("server english")).toBeNull();
    expect((screen.getByTestId(NEXT_ID) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.queryByTestId("setup-step-done-watches")).toBeNull();
    fireEvent.click(screen.getByRole("switch"));
    expect(screen.queryByTestId("watches-error")).toBeNull();
    expect((screen.getByTestId(NEXT_ID) as HTMLButtonElement).disabled).toBe(false);
    click(BACK_ID);
    expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe(PLANNER);
  });
});

describe("CoordinatorSetup refusals", () => {
  it("shows the fallback line for a 400 with no known code", async () => {
    mockSetup.mockRejectedValue(
      new ApiError("bad", 400, { error: "nope", step: "identity", field: "name" }),
    );
    open();
    toReview();
    click(FINISH_ID);
    await waitFor(() => expect(screen.getByTestId("coordinator-name-error")).toBeDefined());
    expect(screen.getByTestId("coordinator-name-error").textContent).toContain(
      "A value on this step was not accepted.",
    );
    expect((screen.getByTestId(NEXT_ID) as HTMLButtonElement).disabled).toBe(true);
    typeName("Planner 2");
    expect((screen.getByTestId(NEXT_ID) as HTMLButtonElement).disabled).toBe(false);
  });

  it("shows a may-do row error beside its row", async () => {
    mockSetup.mockRejectedValue(
      new ApiError("bad", 400, {
        step: "may-do",
        field: "policy.actions.move",
        code: "automatic_not_available",
      }),
    );
    open();
    toReview();
    click(FINISH_ID);
    await waitFor(() => expect(screen.getByTestId("may-do-error-move")).toBeDefined());
    expect((screen.getByTestId(NEXT_ID) as HTMLButtonElement).disabled).toBe(true);
  });

  it("stays on Review with a nothing-created banner for any other answer", async () => {
    mockSetup.mockRejectedValue(new ApiError("boom", 500, null));
    open();
    toReview();
    click(FINISH_ID);
    await waitFor(() => expect(screen.getByTestId("setup-banner-not-created")).toBeDefined());
    expect(screen.getByTestId("setup-review")).toBeDefined();
    expect((screen.getByTestId(FINISH_ID) as HTMLButtonElement).disabled).toBe(false);
    expect(mockReplace).not.toHaveBeenCalled();
  });

  it("says it could not confirm when no answer arrived", async () => {
    mockSetup.mockRejectedValue(new TypeError("Failed to fetch"));
    open();
    toReview();
    click(FINISH_ID);
    await waitFor(() => expect(screen.getByTestId("setup-banner-unconfirmed")).toBeDefined());
    expect((screen.getByTestId(FINISH_ID) as HTMLButtonElement).disabled).toBe(false);
  });
});
