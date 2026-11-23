import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
const state = vi.hoisted(() => ({
  plugins: {
    items: [
      {
        id: "disabled-stl",
        display_name: "Disabled STL",
        status: "disabled",
        ui: { file_viewers: [{ extensions: ["stl"] }] },
      },
      {
        id: "failed-stl",
        display_name: "Failed STL",
        status: "error",
        ui: { file_viewers: [{ extensions: ["stl"] }] },
      },
      {
        id: "obj",
        display_name: "OBJ",
        status: "disabled",
        ui: { file_viewers: [{ extensions: ["obj"] }] },
      },
    ],
  },
}));
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (value: typeof state) => unknown) => select(state),
}));
import { UnavailableFileViewers } from "./unavailable-file-viewers";
afterEach(cleanup);
it("shows matching disabled and failed installations with settings links", () => {
  render(<UnavailableFileViewers path="Model.STL" />);
  expect(screen.getByRole("link", { name: "Disabled STL" }).getAttribute("href")).toBe(
    "/settings/plugins/disabled-stl",
  );
  expect(screen.getByText("Disabled")).toBeDefined();
  expect(screen.getByText("Error")).toBeDefined();
  expect(screen.queryByText("OBJ")).toBeNull();
});
it("shows no installation status for an unsupported extension", () => {
  const { container } = render(<UnavailableFileViewers path="model.unknown" />);
  expect(container.textContent).toBe("");
});
