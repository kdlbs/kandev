import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { QuestionImageViewer } from "./question-image-viewer";

afterEach(cleanup);

it("a second thumbnail cannot release the open viewer's shortcut guard", () => {
  const onInspectionChange = vi.fn();
  const { rerender } = render(
    <QuestionImageViewer
      src="blob:first"
      alt="First layout"
      onInspectionChange={onInspectionChange}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: /First layout/ }));
  expect(onInspectionChange).toHaveBeenLastCalledWith(true);
  onInspectionChange.mockClear();
  rerender(
    <>
      <QuestionImageViewer
        src="blob:first"
        alt="First layout"
        onInspectionChange={onInspectionChange}
      />
      <QuestionImageViewer
        src="blob:second"
        alt="Second layout"
        onInspectionChange={onInspectionChange}
      />
    </>,
  );
  expect(onInspectionChange).not.toHaveBeenCalledWith(false);
});
