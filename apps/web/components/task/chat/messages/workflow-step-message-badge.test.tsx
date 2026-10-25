import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render } from "@testing-library/react";
import { WorkflowStepMessageBadge } from "./workflow-step-message-badge";

afterEach(cleanup);

describe("WorkflowStepMessageBadge", () => {
  it.each(["bg-amber-500", "bg-violet-500"])(
    "keeps the built-in workflow color %s",
    (workflowStepColor) => {
      const { container } = render(
        <WorkflowStepMessageBadge
          workflow={{ stepName: "Review", stepColor: workflowStepColor }}
        />,
      );

      expect(container.querySelector("[data-testid='workflow-message-dot']")?.className).toContain(
        workflowStepColor,
      );
    },
  );
});
