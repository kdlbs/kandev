import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ readBootPayload: vi.fn() }));

vi.mock("@/src/boot-payload", () => ({ readBootPayload: mocks.readBootPayload }));

import { LspLanguageCards } from "./lsp-language-cards";

const props = {
  lspAutoStartLanguages: [],
  lspAutoInstallLanguages: [],
  baselineLspAutoStart: [],
  baselineLspAutoInstall: [],
  lspStatusHiddenLanguages: [],
  baselineLspStatusHiddenLanguages: [],
  toggleAutoStart: vi.fn(),
  toggleAutoInstall: vi.fn(),
  toggleStatusVisibility: vi.fn(),
};

beforeEach(() => {
  mocks.readBootPayload.mockReturnValue({
    runtime: {
      lspAutoInstallPreferenceLanguages: ["typescript", "go", "rust", "python"],
    },
  });
});

afterEach(cleanup);

describe("LSP language install guidance", () => {
  it("renders auto-install prerequisites as visible card content", () => {
    render(<LspLanguageCards {...props} />);

    expect(screen.getByTestId("lsp-auto-install-go")).toBeTruthy();
    expect(screen.getByTestId("lsp-install-guidance-go").textContent).toContain(
      "go install golang.org/x/tools/gopls@latest",
    );
  });

  it("shows every language in task status by default and delegates visibility changes", () => {
    render(<LspLanguageCards {...props} />);

    const visibility = screen.getByTestId("lsp-status-visible-go");
    expect(visibility.getAttribute("data-state")).toBe("checked");
    fireEvent.click(visibility);

    expect(props.toggleStatusVisibility).toHaveBeenCalledWith("go", false);
    expect(screen.getByTestId("lsp-status-visibility-description").textContent).toContain(
      "does not start or stop",
    );
  });

  it("renders a hidden language as not shown in task status", () => {
    render(<LspLanguageCards {...props} lspStatusHiddenLanguages={["go"]} />);

    expect(screen.getByTestId("lsp-status-visible-go").getAttribute("data-state")).toBe(
      "unchecked",
    );
  });
});
