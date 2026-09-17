import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AgentAvatar } from "./agent-avatar";
describe("persona avatars", () => {
  it("uses first and last name initials", () => {
    render(<AgentAvatar name="Chief of staff" />);
    expect(screen.getByText("CS")).toBeTruthy();
  });
  it("uses the selected icon", () => {
    render(<AgentAvatar name="Chief of staff" icon="🧭" />);
    expect(screen.getByText("🧭")).toBeTruthy();
  });
});
