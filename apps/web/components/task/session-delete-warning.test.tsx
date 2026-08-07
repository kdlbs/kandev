import { describe, it, expect, afterEach } from "vitest";
import { render, screen, cleanup } from "@testing-library/react";
import { SessionDeleteWarningLines } from "./session-delete-warning";

afterEach(cleanup);

describe("SessionDeleteWarningLines", () => {
  it("renders nothing when there is no warning data yet", () => {
    const { container } = render(<SessionDeleteWarningLines warning={null} />);
    expect(container.innerHTML).toBe("");
  });

  it("renders nothing for a clean session level with its remote", () => {
    const { container } = render(
      <SessionDeleteWarningLines warning={{ uncommittedFiles: 0, unpushedCommits: 0 }} />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("shows both counts when the session has uncommitted files and unpushed commits", () => {
    render(<SessionDeleteWarningLines warning={{ uncommittedFiles: 3, unpushedCommits: 2 }} />);
    expect(screen.getByTestId("session-delete-uncommitted-warning").textContent).toContain("3");
    expect(screen.getByTestId("session-delete-unpushed-warning").textContent).toContain("2");
  });

  it("shows only the uncommitted-files line when only files are dirty", () => {
    render(<SessionDeleteWarningLines warning={{ uncommittedFiles: 1, unpushedCommits: 0 }} />);
    expect(screen.getByTestId("session-delete-uncommitted-warning")).toBeTruthy();
    expect(screen.queryByTestId("session-delete-unpushed-warning")).toBeNull();
  });

  it("shows only the unpushed-commits line when only commits are unpushed", () => {
    render(<SessionDeleteWarningLines warning={{ uncommittedFiles: 0, unpushedCommits: 4 }} />);
    expect(screen.queryByTestId("session-delete-uncommitted-warning")).toBeNull();
    expect(screen.getByTestId("session-delete-unpushed-warning")).toBeTruthy();
  });
});
