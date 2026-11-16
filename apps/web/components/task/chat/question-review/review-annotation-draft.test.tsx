import { act, cleanup, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import type { ClarificationAnswer, ClarificationAnswerAnnotation } from "@/lib/types/http";
import { ClarificationDraftProvider, useClarificationDraft } from "../clarification-draft-context";

const region: ClarificationAnswerAnnotation = {
  image_id: "after",
  x: 1,
  y: 2,
  width: 3,
  height: 4,
  image_width: 10,
  image_height: 10,
  comment: "Spacing",
};

function Draft({ settled = false }: { settled?: boolean }) {
  const [answers] = useState<Record<string, ClarificationAnswer>>({});
  const draft = useClarificationDraft("source:bundle", answers, settled, {
    questions: [{ id: "q1", optionIds: ["o1"] }],
    questionTotal: 1,
  });
  return (
    <>
      <button onClick={() => draft.setAnnotations({ q1: [region], gone: [region] })}>Draw</button>
      <output data-testid="annotations">{JSON.stringify(draft.annotations)}</output>
    </>
  );
}

function TaskPage({ taskId, settled }: { taskId: string; settled?: boolean }) {
  return (
    <StateProvider>
      <ClarificationDraftProvider taskId={taskId}>
        <Draft key={taskId} settled={settled} />
      </ClarificationDraftProvider>
    </StateProvider>
  );
}

afterEach(cleanup);

// @covers AC-TASKS-QUESTION-IMAGE-REVIEW-007.3
it("retains regions with the answer draft across task navigation, for known questions only", () => {
  const { rerender } = render(<TaskPage taskId="task-a" />);
  act(() => screen.getByRole("button", { name: "Draw" }).click());
  expect(screen.getByTestId("annotations").textContent).toBe(JSON.stringify({ q1: [region] }));
  rerender(<TaskPage taskId="task-b" />);
  expect(screen.getByTestId("annotations").textContent).toBe("{}");
  rerender(<TaskPage taskId="task-a" />);
  expect(screen.getByTestId("annotations").textContent).toBe(JSON.stringify({ q1: [region] }));
});
