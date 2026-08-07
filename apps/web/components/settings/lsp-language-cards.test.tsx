import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const mocks = vi.hoisted(() => ({ readBootPayload: vi.fn() }));

vi.mock("@/src/boot-payload", () => ({ readBootPayload: mocks.readBootPayload }));

import { LspLanguageCards } from "./lsp-language-cards";

const props = {
  lspAutoStartLanguages: [],
  lspAutoInstallLanguages: [],
  baselineLspAutoStart: [],
  baselineLspAutoInstall: [],
  toggleAutoStart: vi.fn(),
  toggleAutoInstall: vi.fn(),
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
});
