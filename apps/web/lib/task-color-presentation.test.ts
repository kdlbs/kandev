import { describe, expect, it } from "vitest";
import {
  fixedAutomaticTaskColor,
  parseWorkflowStepColor,
  taskColorPresentation,
} from "./task-color-presentation";

describe("task color presentation", () => {
  it("maps fixed automatic colors through the static safe registry", () => {
    expect(fixedAutomaticTaskColor("gray")).toEqual({
      token: "gray",
      className: "bg-slate-500",
    });
    expect(fixedAutomaticTaskColor("cyan")).toEqual({
      token: "cyan",
      className: "bg-cyan-500",
    });
  });

  it("maps workflow catalog and legacy classes without rendering arbitrary classes", () => {
    expect(parseWorkflowStepColor("emerald")).toEqual(taskColorPresentation("green"));
    expect(parseWorkflowStepColor("bg-neutral-400")).toEqual(taskColorPresentation("gray"));
    expect(parseWorkflowStepColor("bg-emerald-500")).toEqual(taskColorPresentation("green"));
    expect(parseWorkflowStepColor("bg-[url(http://evil)]")).toEqual(taskColorPresentation("gray"));
  });

  it("accepts strict hex values as inline presentation styles", () => {
    expect(parseWorkflowStepColor("#0af")).toEqual({
      token: "custom",
      style: { backgroundColor: "#0af" },
    });
    expect(parseWorkflowStepColor("#00aaff88")).toEqual({
      token: "custom",
      style: { backgroundColor: "#00aaff88" },
    });
    expect(parseWorkflowStepColor("#00afg0")).toEqual(taskColorPresentation("gray"));
  });
});
