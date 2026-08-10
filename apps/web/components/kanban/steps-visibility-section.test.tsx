import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StepsVisibilitySection } from "./steps-visibility-section";
import { useStepsDisclosureOverrides } from "@/hooks/use-steps-disclosure-overrides";

afterEach(cleanup);

const WF_A = { id: "wf-a", name: "Workflow A" };
const WF_B = { id: "wf-b", name: "Workflow B" };
const STEP_A1_TESTID = "steps-filter-step-a1";

function groupToggle(workflowId: string) {
  return screen.getByTestId(`steps-filter-group-toggle-${workflowId}`);
}

function expectExpanded(workflowId: string, expanded: boolean) {
  expect(groupToggle(workflowId).getAttribute("aria-expanded")).toBe(String(expanded));
}

function baseProps(overrides: Partial<React.ComponentProps<typeof StepsVisibilitySection>> = {}) {
  return {
    eligibleWorkflows: [WF_A],
    snapshots: {},
    hiddenWorkflowStepIds: {},
    onToggleStepVisibility: vi.fn(),
    overrides: {},
    onToggleGroupDisclosure: vi.fn(),
    ...overrides,
  };
}

describe("StepsVisibilitySection — group/step derivation and ordering (item 1)", () => {
  it("orders steps by ascending position, tiebreaking by ascending id", () => {
    render(
      <StepsVisibilitySection
        {...baseProps({
          snapshots: {
            [WF_A.id]: {
              steps: [
                { id: "step-z", title: "Z step", position: 1 },
                { id: "step-a", title: "A step", position: 0 },
                { id: "step-b-tie", title: "B tie", position: 1 },
              ],
            } as never,
          },
        })}
      />,
    );

    const group = screen.getByTestId(`steps-filter-group-${WF_A.id}`);
    const rows = Array.from(group.querySelectorAll('[data-testid^="steps-filter-step-row-"]'));
    expect(rows.map((row) => row.textContent)).toEqual(["A step", "B tie", "Z step"]);
  });

  it("renders nothing when there are no eligible workflows", () => {
    const { container } = render(
      <StepsVisibilitySection {...baseProps({ eligibleWorkflows: [] })} />,
    );
    expect(container.firstChild).toBeNull();
  });
});

describe("StepsVisibilitySection — single-workflow rule wins (R2)", () => {
  it("renders inline with no header, identical to R1, for exactly one eligible workflow", () => {
    render(
      <StepsVisibilitySection
        {...baseProps({
          snapshots: {
            [WF_A.id]: { steps: [{ id: "s1", title: "Step 1", position: 0 }] } as never,
          },
        })}
      />,
    );
    expect(screen.queryByTestId(`steps-filter-group-toggle-${WF_A.id}`)).toBeNull();
    expect(screen.getByTestId("steps-filter-step-s1")).not.toBeNull();
    expect(screen.getByTestId("steps-filter-step-row-s1")).not.toBeNull();
  });

  it("still renders label/description with no rows and no summary for a sole zero-step workflow", () => {
    render(
      <StepsVisibilitySection
        {...baseProps({ snapshots: { [WF_A.id]: { steps: [] } as never } })}
      />,
    );
    expect(screen.getByText("Steps")).not.toBeNull();
    expect(screen.queryByTestId(`steps-filter-group-toggle-${WF_A.id}`)).toBeNull();
    expect(screen.queryByTestId(/^steps-filter-step-/)).toBeNull();
    expect(screen.getByTestId(`steps-filter-group-${WF_A.id}`)).not.toBeNull();
  });
});

describe("StepsVisibilitySection — multi-workflow disclosure headers and default expansion", () => {
  const snapshots = {
    [WF_A.id]: { steps: [{ id: "a1", title: "A1", position: 0 }] },
    [WF_B.id]: { steps: [{ id: "b1", title: "B1", position: 0 }] },
  } as never;

  it("shows a header with aria-expanded for every group when more than one workflow is eligible", () => {
    render(
      <StepsVisibilitySection {...baseProps({ eligibleWorkflows: [WF_A, WF_B], snapshots })} />,
    );
    expect(screen.getByTestId(`steps-filter-group-toggle-${WF_A.id}`)).not.toBeNull();
    expect(screen.getByTestId(`steps-filter-group-toggle-${WF_B.id}`)).not.toBeNull();
  });

  it("defaults to collapsed with an empty hidden set — no checkbox in the DOM", () => {
    render(
      <StepsVisibilitySection {...baseProps({ eligibleWorkflows: [WF_A, WF_B], snapshots })} />,
    );
    expectExpanded(WF_A.id, false);
    expect(screen.queryByTestId(STEP_A1_TESTID)).toBeNull();
    // Container is still present even when collapsed.
    expect(screen.getByTestId(`steps-filter-group-${WF_A.id}`)).not.toBeNull();
  });

  it("defaults to expanded for a group holding a live hidden step", () => {
    render(
      <StepsVisibilitySection
        {...baseProps({
          eligibleWorkflows: [WF_A, WF_B],
          snapshots,
          hiddenWorkflowStepIds: { [WF_A.id]: ["a1"] },
        })}
      />,
    );
    expectExpanded(WF_A.id, true);
    expect(screen.getByTestId(STEP_A1_TESTID)).not.toBeNull();
    // The other workflow, with nothing hidden, stays collapsed.
    expectExpanded(WF_B.id, false);
  });

  it("expanding a group reveals exactly that workflow's steps in position order and toggles disclosure only for that workflow", () => {
    const onToggleGroupDisclosure = vi.fn();
    render(
      <StepsVisibilitySection
        {...baseProps({ eligibleWorkflows: [WF_A, WF_B], snapshots, onToggleGroupDisclosure })}
      />,
    );
    fireEvent.click(groupToggle(WF_A.id));
    expect(onToggleGroupDisclosure).toHaveBeenCalledWith(WF_A.id, false);
  });

  it("clicking a step checkbox calls onToggleStepVisibility with the workflow and step id", () => {
    const onToggleStepVisibility = vi.fn();
    render(
      <StepsVisibilitySection
        {...baseProps({
          eligibleWorkflows: [WF_A, WF_B],
          snapshots,
          hiddenWorkflowStepIds: { [WF_A.id]: ["a1"] },
          onToggleStepVisibility,
        })}
      />,
    );
    fireEvent.click(screen.getByTestId(STEP_A1_TESTID));
    expect(onToggleStepVisibility).toHaveBeenCalledWith(WF_A.id, "a1");
  });
});

describe("StepsVisibilitySection — shown-count derivation (item 8)", () => {
  it("reads 0 of 0 shown for a zero-step workflow when a header is rendered", () => {
    render(
      <StepsVisibilitySection
        {...baseProps({
          eligibleWorkflows: [WF_A, WF_B],
          snapshots: {
            [WF_A.id]: { steps: [] },
            [WF_B.id]: { steps: [{ id: "b1", title: "B1", position: 0 }] },
          } as never,
        })}
      />,
    );
    expect(groupToggle(WF_A.id).textContent).toContain("0 of 0 shown");
  });

  it("excludes a stale hidden id from both the hidden count and the total", () => {
    render(
      <StepsVisibilitySection
        {...baseProps({
          eligibleWorkflows: [WF_A, WF_B],
          snapshots: {
            [WF_A.id]: {
              steps: [
                { id: "a1", title: "A1", position: 0 },
                { id: "a2", title: "A2", position: 1 },
              ],
            },
            [WF_B.id]: { steps: [{ id: "b1", title: "B1", position: 0 }] },
          } as never,
          hiddenWorkflowStepIds: { [WF_A.id]: ["a1", "stale-id"] },
        })}
      />,
    );
    // 2 live steps total, 1 live hidden (a1) — stale-id counted in neither.
    expect(groupToggle(WF_A.id).textContent).toContain("1 of 2 shown");
  });
});

describe("StepsVisibilitySection — long-title truncation (item 9)", () => {
  const LONG_STEP_TITLE =
    "A very long step title that should truncate instead of widening the drawer";
  const LONG_WORKFLOW_NAME =
    "A very long workflow name that should also truncate on the group header";

  it("truncates a long step title to a single line and carries the full text in `title`", () => {
    render(
      <StepsVisibilitySection
        {...baseProps({
          snapshots: {
            [WF_A.id]: { steps: [{ id: "s1", title: LONG_STEP_TITLE, position: 0 }] },
          } as never,
        })}
      />,
    );
    const row = screen.getByTestId("steps-filter-step-row-s1");
    const label = row.querySelector("span[title]");
    expect(label?.className).toContain("truncate");
    expect(label?.getAttribute("title")).toBe(LONG_STEP_TITLE);
    expect(label?.textContent).toBe(LONG_STEP_TITLE);
  });

  it("truncates a long workflow name on the group header and carries the full text in `title`", () => {
    render(
      <StepsVisibilitySection
        {...baseProps({
          eligibleWorkflows: [{ id: WF_A.id, name: LONG_WORKFLOW_NAME }, WF_B],
          snapshots: {
            [WF_A.id]: { steps: [{ id: "a1", title: "A1", position: 0 }] },
            [WF_B.id]: { steps: [{ id: "b1", title: "B1", position: 0 }] },
          } as never,
        })}
      />,
    );
    const nameSpan = groupToggle(WF_A.id).querySelector("span[title]");
    expect(nameSpan?.className).toContain("truncate");
    expect(nameSpan?.getAttribute("title")).toBe(LONG_WORKFLOW_NAME);
  });
});

// item 7: override survival across an eligible-count change (many -> 1 -> many).
describe("StepsVisibilitySection — override survival across an eligible-count change (item 7)", () => {
  function Harness({ manyWorkflows }: { manyWorkflows: boolean }) {
    const [hidden] = useState<Record<string, string[]>>({});
    const { overrides, toggleDisclosure } = useStepsDisclosureOverrides(true, "mobile");
    const snapshots = {
      [WF_A.id]: { steps: [{ id: "a1", title: "A1", position: 0 }] },
      [WF_B.id]: { steps: [{ id: "b1", title: "B1", position: 0 }] },
    } as never;
    return (
      <StepsVisibilitySection
        eligibleWorkflows={manyWorkflows ? [WF_A, WF_B] : [WF_A]}
        snapshots={snapshots}
        hiddenWorkflowStepIds={hidden}
        onToggleStepVisibility={vi.fn()}
        overrides={overrides}
        onToggleGroupDisclosure={toggleDisclosure}
      />
    );
  }

  it("a many->1->many transition does not clear or consult A's recorded override", () => {
    const { rerender } = render(<Harness manyWorkflows />);

    // Explicitly expand A (collapsed default, nothing hidden).
    fireEvent.click(groupToggle(WF_A.id));
    expectExpanded(WF_A.id, true);

    // Drop to a single eligible workflow: renders inline, fully expanded,
    // regardless of the recorded override.
    rerender(<Harness manyWorkflows={false} />);
    expect(screen.queryByTestId(`steps-filter-group-toggle-${WF_A.id}`)).toBeNull();
    expect(screen.getByTestId(STEP_A1_TESTID)).not.toBeNull();

    // Back to many: the override recorded earlier still applies.
    rerender(<Harness manyWorkflows />);
    expectExpanded(WF_A.id, true);
  });
});

// The re-tick coupling rules: a step toggle never overrides the user's own
// disclosure choice, but it may move the recomputed default.
describe("StepsVisibilitySection — step toggle vs disclosure coupling", () => {
  function Harness() {
    const [hidden, setHidden] = useState<Record<string, string[]>>({ [WF_A.id]: ["a1"] });
    const { overrides, toggleDisclosure } = useStepsDisclosureOverrides(true, "mobile");
    const snapshots = {
      [WF_A.id]: { steps: [{ id: "a1", title: "A1", position: 0 }] },
      [WF_B.id]: { steps: [{ id: "b1", title: "B1", position: 0 }] },
    } as never;
    const onToggleStepVisibility = (workflowId: string, stepId: string) => {
      setHidden((prev) => {
        const current = prev[workflowId] ?? [];
        const next = current.includes(stepId)
          ? current.filter((id) => id !== stepId)
          : [...current, stepId];
        return { ...prev, [workflowId]: next };
      });
    };
    return (
      <StepsVisibilitySection
        eligibleWorkflows={[WF_A, WF_B]}
        snapshots={snapshots}
        hiddenWorkflowStepIds={hidden}
        onToggleStepVisibility={onToggleStepVisibility}
        overrides={overrides}
        onToggleGroupDisclosure={toggleDisclosure}
      />
    );
  }

  it("re-ticking the last hidden step with NO override collapses the group (recomputed default reasserts)", () => {
    render(<Harness />);
    // A starts expanded by default (a1 is hidden).
    expectExpanded(WF_A.id, true);

    fireEvent.click(screen.getByTestId(STEP_A1_TESTID));

    expectExpanded(WF_A.id, false);
  });

  it("re-ticking the last hidden step WITH an explicit override stays expanded", () => {
    render(<Harness />);
    // Explicitly collapse then re-expand A, recording an override of `true`.
    fireEvent.click(groupToggle(WF_A.id));
    fireEvent.click(groupToggle(WF_A.id));
    expectExpanded(WF_A.id, true);

    fireEvent.click(screen.getByTestId(STEP_A1_TESTID));

    expectExpanded(WF_A.id, true);
    expect(screen.getByTestId(STEP_A1_TESTID)).not.toBeNull();
  });

  it("unticking a step inside an explicitly expanded group leaves it expanded", () => {
    render(<Harness />);
    // Re-tick a1 first so the group starts at the collapsed default (nothing hidden).
    fireEvent.click(screen.getByTestId(STEP_A1_TESTID));
    expectExpanded(WF_A.id, false);
    // Explicitly expand A.
    fireEvent.click(groupToggle(WF_A.id));
    expectExpanded(WF_A.id, true);

    // Unticking a step inside A must not collapse it.
    fireEvent.click(screen.getByTestId(STEP_A1_TESTID));

    expectExpanded(WF_A.id, true);
    expect(screen.getByTestId(STEP_A1_TESTID)).not.toBeNull();
  });
});
