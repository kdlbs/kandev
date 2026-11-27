import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { WebSocketRequestError } from "@/lib/ws/request-error";
import {
  AutomationCredentialErrorMessage,
  parseAutomationCredentialError,
} from "./automation-credential-error";

afterEach(cleanup);

describe("automation credential errors", () => {
  it.each([
    ["agent profile", "Codex token", "agentProfileId"],
    ["executor profile", "SSH token", "executorProfileId"],
    ["repository app", "NPM_TOKEN", "repositorySelections"],
  ] as const)("maps %s to its editor field", (source, field, target) => {
    const error = new WebSocketRequestError("unavailable", "VALIDATION_ERROR", {
      credential_error_code: "automation_credentials_unavailable",
      source,
      field,
    });

    expect(parseAutomationCredentialError(error)).toEqual({ source, field, target });
  });

  it("announces safe recovery text at the affected field", () => {
    const error = parseAutomationCredentialError(
      new WebSocketRequestError("unavailable", "VALIDATION_ERROR", {
        credential_error_code: "automation_credentials_unavailable",
        source: "agent profile",
        field: "SERVICE_KEY",
      }),
    );

    render(<AutomationCredentialErrorMessage error={error!} />);

    const alert = screen.getByRole("alert");
    expect(alert.textContent).toContain("SERVICE_KEY");
    expect(alert.textContent).toContain("Choose another profile.");
    expect(screen.queryByText(/secret-id/i)).toBeNull();
  });
});
