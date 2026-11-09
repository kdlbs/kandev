import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { ClarificationQuestion } from "@/lib/types/http";
import { QuestionImageGallery } from "./question-image-gallery";
import { QuestionImageViewer } from "./question-image-viewer";

afterEach(cleanup);

const COUNTER = "question-image-counter";
const NEXT = "question-image-next";

const images = [
  { id: "before", path: "before.png", alt: "Before", label: "before" },
  { id: "after", path: "after.png", alt: "After" },
  { id: "alt", path: "alt.png", alt: "Alternative" },
];

const question: ClarificationQuestion = {
  id: "q1",
  title: "",
  prompt: "Pick one",
  images: images.slice(0, 2),
  options: [
    { option_id: "o1", label: "Alt", description: "", images: [images[2]] },
    { option_id: "o2", label: "None", description: "" },
  ],
};

function renderGallery() {
  return render(
    <QuestionImageGallery question={question}>
      {images.map((image) => (
        <QuestionImageViewer
          key={image.id}
          imageId={image.id}
          src={`blob:${image.id}`}
          alt={image.alt}
        />
      ))}
    </QuestionImageGallery>,
  );
}

describe("thumbnail inspector gallery", () => {
  // @covers AC-TASKS-QUESTION-IMAGES-003.6
  it("steps through the question's images with buttons and arrow keys, without wrapping", async () => {
    renderGallery();
    fireEvent.click(screen.getByRole("button", { name: /After/ }));
    const dialog = screen.getByRole("dialog");
    expect(within(dialog).getByTestId(COUNTER).textContent).toBe("2 of 3");
    fireEvent.click(within(dialog).getByTestId(NEXT));
    expect(within(dialog).getByRole("img").getAttribute("src")).toBe("blob:alt");
    expect(within(dialog).getByTestId(NEXT)).toHaveProperty("disabled", true);
    fireEvent.keyDown(dialog, { key: "ArrowLeft" });
    fireEvent.keyDown(dialog, { key: "ArrowLeft" });
    expect(within(dialog).getByTestId(COUNTER).textContent).toBe("before, 1 of 3");
    expect(within(dialog).getByTestId("question-image-previous")).toHaveProperty("disabled", true);
  });

  it("returns focus to the thumbnail of the image that was showing", async () => {
    renderGallery();
    fireEvent.click(screen.getByRole("button", { name: /Before/ }));
    const dialog = screen.getByRole("dialog");
    fireEvent.click(within(dialog).getByTestId(NEXT));
    fireEvent.keyDown(dialog, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).toBe(screen.getByRole("button", { name: /After/ }));
  });

  it("shows no navigation for a single image", () => {
    render(<QuestionImageViewer src="blob:one" alt="Only" />);
    fireEvent.click(screen.getByRole("button", { name: /Only/ }));
    expect(screen.queryByTestId(NEXT)).toBeNull();
  });
});
