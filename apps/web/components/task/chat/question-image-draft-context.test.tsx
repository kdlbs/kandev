import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { QuestionImageDraftProvider, useQuestionImageDraft } from "./question-image-draft-context";

const answers = { q1: { question_id: "q1", selected_options: ["yes"] } };
function Draft({ bundle = "source:bundle", retain = true, settled = false }) {
  const draft = useQuestionImageDraft(bundle, retain, answers, settled);
  return (
    <>
      <button
        onClick={() => {
          draft.setCustomDrafts({ q2: "Keep this" });
          draft.setActiveIndex(1);
        }}
      >
        Edit
      </button>
      <output data-testid="draft">
        {JSON.stringify([draft.customDrafts, draft.rawActiveIndex, draft.savedAnswers])}
      </output>
    </>
  );
}
function Page({
  mount,
  bundle,
  retain = true,
  settled = false,
}: {
  mount: string;
  bundle?: string;
  retain?: boolean;
  settled?: boolean;
}) {
  return (
    <QuestionImageDraftProvider>
      <Draft key={mount} bundle={bundle} retain={retain} settled={settled} />
    </QuestionImageDraftProvider>
  );
}
afterEach(cleanup);

it("retains text, position, and keyed choices across a layout remount", () => {
  const { rerender } = render(<Page mount="desktop" />);
  act(() => screen.getByRole("button").click());
  rerender(<Page mount="phone" />);
  expect(screen.getByTestId("draft").textContent).toBe(
    JSON.stringify([{ q2: "Keep this" }, 1, [answers.q1]]),
  );
});

it("does not reuse a draft for another source or bundle", () => {
  const { rerender } = render(<Page mount="desktop" />);
  act(() => screen.getByRole("button").click());
  rerender(<Page mount="phone" bundle="other:bundle" />);
  expect(screen.getByTestId("draft").textContent).toBe(JSON.stringify([{}, 0, undefined]));
});

it("clears a settled bundle and leaves image-free forms local", () => {
  const { rerender } = render(<Page mount="desktop" />);
  act(() => screen.getByRole("button").click());
  rerender(<Page mount="desktop" settled />);
  rerender(<Page mount="phone" settled />);
  expect(screen.getByTestId("draft").textContent).toBe(JSON.stringify([{}, 0, undefined]));
  rerender(<Page mount="legacy" bundle="legacy" retain={false} />);
  act(() => screen.getByRole("button").click());
  rerender(<Page mount="legacy-phone" bundle="legacy" retain={false} />);
  expect(screen.getByTestId("draft").textContent).toBe(JSON.stringify([{}, 0, undefined]));
});
