import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/client";
import {
  actionOfPath,
  activeSetupSteps,
  buildSetupRequest,
  contextReviewValue,
  goalErrors,
  gotRefusal,
  initialSetupState,
  isGoalEmpty,
  isSetupValid,
  isStepValid,
  setupServerError,
  skipStep,
  type SetupState,
} from "./setup";

function valid(): SetupState {
  return {
    ...initialSetupState({ agentProfileId: "a1", executorProfileId: "e1" }),
    name: " Planner ",
  };
}

const criterion = (text: string) => ({ key: text, text, done: false });

describe("initialSetupState", () => {
  it("watches every board and asks approval for four actions", () => {
    const state = initialSetupState({ agentProfileId: "", executorProfileId: "" });
    expect(state.watches).toEqual({ scope: "all", workflowIds: [] });
    expect(state.actions).toEqual({
      create_task: "requires_approval",
      start_agent: "denied",
      message: "requires_approval",
      move: "requires_approval",
      resume: "requires_approval",
      stop: "denied",
    });
  });
});

describe("step validity", () => {
  it("needs a trimmed name of 1 to 60 code points and both profiles", () => {
    const state = valid();
    expect(isStepValid("identity", state)).toBe(true);
    expect(isStepValid("identity", { ...state, name: "   " })).toBe(false);
    expect(isStepValid("identity", { ...state, name: "😀".repeat(60) })).toBe(true);
    expect(isStepValid("identity", { ...state, name: "😀".repeat(61) })).toBe(false);
    expect(isStepValid("identity", { ...state, agentProfileId: "" })).toBe(false);
    expect(isStepValid("identity", { ...state, executorProfileId: "" })).toBe(false);
  });

  it("needs at least one board for a selected set", () => {
    const state = valid();
    expect(isStepValid("watches", state)).toBe(true);
    const none = { ...state, watches: { scope: "selected" as const, workflowIds: [] } };
    expect(isStepValid("watches", none)).toBe(false);
    expect(
      isStepValid("watches", { ...none, watches: { ...none.watches, workflowIds: ["w"] } }),
    ).toBe(true);
  });

  it("treats whitespace-only context as empty and judges the trimmed length", () => {
    const state = valid();
    expect(isStepValid("context", { ...state, context: "   \n " })).toBe(true);
    expect(isStepValid("context", { ...state, context: ` ${"x".repeat(4000)} ` })).toBe(true);
    expect(isStepValid("context", { ...state, context: "x".repeat(4001) })).toBe(false);
  });

  it("is valid overall only when every step is", () => {
    expect(isSetupValid(valid())).toBe(true);
    expect(isSetupValid({ ...valid(), name: "" })).toBe(false);
  });
});

describe("goal validity", () => {
  it("is empty only with no name, due date or criterion row", () => {
    expect(isGoalEmpty({ name: "  ", dueOn: "", criteria: [] })).toBe(true);
    expect(isGoalEmpty({ name: "", dueOn: "2026-01-01", criteria: [] })).toBe(false);
    expect(isGoalEmpty({ name: "", dueOn: "", criteria: [criterion("")] })).toBe(false);
  });

  it("flags a name, a date that is not real and a blank or long criterion by path", () => {
    const errors = goalErrors({
      name: "",
      dueOn: "2026-02-30",
      criteria: [criterion("ok"), criterion("  "), criterion("x".repeat(201))],
    });
    expect(Object.keys(errors).sort()).toEqual([
      "goal.criteria[1].text",
      "goal.criteria[2].text",
      "goal.due_on",
      "goal.name",
    ]);
  });

  it("allows at most 10 criteria", () => {
    const criteria = Array.from({ length: 11 }, (_, i) => criterion(`c${i}`));
    expect(goalErrors({ name: "m", dueOn: "", criteria })["goal.criteria"]).toBeDefined();
    expect(goalErrors({ name: "m", dueOn: "", criteria: criteria.slice(0, 10) })).toEqual({});
  });
});

describe("skipStep", () => {
  it("clears the goal and the context and nothing else", () => {
    const state = { ...valid(), context: "text", goal: { name: "m", dueOn: "", criteria: [] } };
    expect(skipStep("goal", state).goal).toEqual({ name: "", dueOn: "", criteria: [] });
    expect(skipStep("goal", state).context).toBe("text");
    expect(skipStep("context", state).context).toBe("");
    expect(skipStep("watches", state)).toBe(state);
  });
});

describe("buildSetupRequest", () => {
  it("sends trimmed values, no goal when skipped and no ids for every board", () => {
    const request = buildSetupRequest({ ...valid(), context: "  hi  " });
    expect(request).toEqual({
      name: "Planner",
      agent_profile_id: "a1",
      executor_profile_id: "e1",
      context: "hi",
      watches: { scope: "all" },
      policy: { actions: valid().actions },
    });
    expect("goal" in request).toBe(false);
  });

  it("sends a selected set and a goal whose criteria carry no id", () => {
    const request = buildSetupRequest({
      ...valid(),
      watches: { scope: "selected", workflowIds: ["w1", "w2"] },
      goal: { name: " Ship ", dueOn: "", criteria: [criterion(" one ")] },
    });
    expect(request.watches).toEqual({ scope: "selected", workflow_ids: ["w1", "w2"] });
    expect(request.goal).toEqual({ name: "Ship", due_on: null, criteria: [{ text: "one" }] });
  });
});

describe("contextReviewValue", () => {
  it("is null for empty or whitespace-only text", () => {
    expect(contextReviewValue("  \n ")).toBeNull();
  });

  it("keeps the first line and adds an ellipsis when more lines follow", () => {
    expect(contextReviewValue("one\ntwo")).toBe("one…");
    expect(contextReviewValue("  one  ")).toBe("one");
  });

  it("cuts at 80 code points", () => {
    expect(contextReviewValue("😀".repeat(80))).toBe("😀".repeat(80));
    expect(contextReviewValue("😀".repeat(81))).toBe(`${"😀".repeat(80)}…`);
  });
});

describe("server answers", () => {
  it("reads step, field and code from a setup 400", () => {
    const error = new ApiError("bad", 400, {
      step: "watches",
      field: "watches",
      code: "watches_empty",
    });
    expect(setupServerError(error)).toEqual({
      step: "watches",
      field: "watches",
      code: "watches_empty",
    });
  });

  it("is not a step error without a step or for another status", () => {
    expect(setupServerError(new ApiError("bad", 400, { field: "name" }))).toBeNull();
    expect(setupServerError(new ApiError("bad", 500, { step: "identity" }))).toBeNull();
    expect(setupServerError(new TypeError("network"))).toBeNull();
  });

  it("counts only an error status as a refusal", () => {
    expect(gotRefusal(new ApiError("x", 500, null))).toBe(true);
    expect(gotRefusal(new ApiError("x", 201, null))).toBe(false);
    expect(gotRefusal(new TypeError("network"))).toBe(false);
  });

  it("maps a policy path to one of the six actions", () => {
    expect(actionOfPath("policy.actions.stop")).toBe("stop");
    expect(actionOfPath("policy.actions.bogus")).toBeNull();
    expect(actionOfPath("policy")).toBeNull();
  });
});

describe("projects step", () => {
  const selected = (patch: Partial<SetupState["projects"]>): SetupState => ({
    ...valid(),
    projects: { scope: "selected", entries: [], includeNoRepository: false, ...patch },
  });

  it("is offered only while the flag is effective", () => {
    expect(activeSetupSteps(false)).not.toContain("projects");
    const steps = activeSetupSteps(true);
    expect(steps.indexOf("projects")).toBe(steps.indexOf("watches") + 1);
  });

  it("sends no projects member under scope all and the entries once selected", () => {
    expect(buildSetupRequest(valid())).not.toHaveProperty("projects");
    const entries = [{ kind: "repository_set" as const, id: "s1" }];
    expect(buildSetupRequest(selected({ entries })).projects).toEqual({
      scope: "selected",
      entries,
      include_no_repository: false,
    });
  });

  it("needs an entry or the no-repository toggle, and at most 50 entries", () => {
    expect(isStepValid("projects", valid())).toBe(true);
    expect(isStepValid("projects", selected({}))).toBe(false);
    expect(isStepValid("projects", selected({ includeNoRepository: true }))).toBe(true);
    const many = Array.from({ length: 51 }, (_, i) => ({
      kind: "repository" as const,
      id: `r${i}`,
    }));
    expect(isStepValid("projects", selected({ entries: many }))).toBe(false);
  });

  it("Skip returns to scope all", () => {
    const skipped = skipStep("projects", selected({ includeNoRepository: true }));
    expect(skipped.projects).toEqual({ scope: "all", entries: [], includeNoRepository: false });
  });

  it("reads a server refusal naming the projects step", () => {
    const error = new ApiError("bad", 400, {
      step: "projects",
      field: "projects",
      code: "projects_empty",
    });
    expect(setupServerError(error)).toEqual({
      step: "projects",
      field: "projects",
      code: "projects_empty",
    });
  });
});
