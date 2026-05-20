/**
 * Tests for TaskMetaRail — the right-rail strategy switch.
 *
 * Covers all four (style, stage_type) combinations from F7.3:
 *   review/approval stage   -> multi-agent
 *   workflow.style=office   -> office
 *   workflow.style=kanban   -> workflow (default)
 *   undefined / unknown     -> workflow (graceful fallback)
 */

import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { TaskMetaRail, pickMetaRailVariant } from "./TaskMetaRail";

afterEach(cleanup);

const MULTI_AGENT = "multi-agent";

describe("pickMetaRailVariant", () => {
  it("review stage forces multi-agent regardless of style", () => {
    expect(pickMetaRailVariant("kanban", "review")).toBe(MULTI_AGENT);
    expect(pickMetaRailVariant("office", "review")).toBe(MULTI_AGENT);
    expect(pickMetaRailVariant("custom", "review")).toBe(MULTI_AGENT);
  });

  it("approval stage forces multi-agent regardless of style", () => {
    expect(pickMetaRailVariant("kanban", "approval")).toBe(MULTI_AGENT);
    expect(pickMetaRailVariant("office", "approval")).toBe(MULTI_AGENT);
  });

  it("office style picks office variant when not in a review/approval stage", () => {
    expect(pickMetaRailVariant("office", "work")).toBe("office");
    expect(pickMetaRailVariant("office", "custom")).toBe("office");
    expect(pickMetaRailVariant("office", null)).toBe("office");
  });

  it("kanban style picks the default workflow variant", () => {
    expect(pickMetaRailVariant("kanban", "work")).toBe("workflow");
    expect(pickMetaRailVariant("kanban", "custom")).toBe("workflow");
    expect(pickMetaRailVariant("kanban", null)).toBe("workflow");
  });

  it("undefined / null inputs fall back to workflow", () => {
    expect(pickMetaRailVariant(null, null)).toBe("workflow");
    expect(pickMetaRailVariant(undefined, undefined)).toBe("workflow");
  });
});

describe("TaskMetaRail", () => {
  const slots = {
    multiAgentSlot: <div>multi-agent-rail</div>,
    officeSlot: <div>office-rail</div>,
    workflowSlot: <div>workflow-rail</div>,
  };

  it("renders multi-agent rail for review stage", () => {
    render(<TaskMetaRail workflowStyle="office" stageType="review" {...slots} />);
    expect(screen.getByText("multi-agent-rail")).toBeTruthy();
    expect(screen.queryByText("office-rail")).toBeNull();
    expect(screen.queryByText("workflow-rail")).toBeNull();
  });

  it("renders multi-agent rail for approval stage", () => {
    render(<TaskMetaRail workflowStyle="kanban" stageType="approval" {...slots} />);
    expect(screen.getByText("multi-agent-rail")).toBeTruthy();
  });

  it("renders office rail for office style on a non-review stage", () => {
    render(<TaskMetaRail workflowStyle="office" stageType="work" {...slots} />);
    expect(screen.getByText("office-rail")).toBeTruthy();
  });

  it("renders workflow rail for kanban style", () => {
    render(<TaskMetaRail workflowStyle="kanban" stageType="work" {...slots} />);
    expect(screen.getByText("workflow-rail")).toBeTruthy();
  });

  it("renders workflow rail when style and stage are unknown", () => {
    render(<TaskMetaRail {...slots} />);
    expect(screen.getByText("workflow-rail")).toBeTruthy();
  });

  it("renders nothing when the chosen slot is missing", () => {
    const { container } = render(<TaskMetaRail workflowStyle="office" stageType="review" />);
    expect(container.textContent).toBe("");
  });
});
