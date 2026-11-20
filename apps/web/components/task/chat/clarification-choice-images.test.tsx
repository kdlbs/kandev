import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { ClarificationOptions } from "./clarification-overlay-parts";
import { QuestionImageViewer } from "./question-image-viewer";

afterEach(cleanup);

it("keeps choice inspection separate from selection and submission state", () => {
  const onSelectOption = vi.fn();
  const onInspectionChange = vi.fn();
  const options = [{ option_id: "wide", label: "Wide layout", description: "Roomy spacing" }];
  const props = {
    options,
    selectedOption: null,
    isSubmitting: false,
    customActive: true,
    onSelectOption,
    renderOptionPreview: () => (
      <QuestionImageViewer
        src="blob:wide"
        alt="Wide preview"
        onInspectionChange={onInspectionChange}
      />
    ),
  };
  const { rerender } = render(<ClarificationOptions {...props} />);
  const inspect = screen.getByRole("button", { name: /Wide preview/ });
  expect(inspect.parentElement?.closest("button")).toBeNull();
  fireEvent.click(inspect);
  expect(screen.getByRole("dialog", { name: "Wide preview" })).toBeTruthy();
  expect(onInspectionChange).toHaveBeenLastCalledWith(true);
  expect(onSelectOption).not.toHaveBeenCalled();
  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: "Close" }));
  expect(onSelectOption).not.toHaveBeenCalled();
  fireEvent.click(screen.getByTestId("clarification-option"));
  expect(onSelectOption).toHaveBeenCalledExactlyOnceWith("wide");
  rerender(<ClarificationOptions {...props} isSubmitting />);
  fireEvent.click(screen.getByTestId("clarification-option"));
  expect(onSelectOption).toHaveBeenCalledTimes(1);
});
