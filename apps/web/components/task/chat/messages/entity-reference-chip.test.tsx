import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EntityReference } from "@/lib/types/entity-reference";
import { EntityReferenceChip } from "./entity-reference-chip";

const actTaskChangeRegistration = vi.hoisted(() => vi.fn());
const TASK_ID = "task-1";
const SELECTION_ID = "selection-1";
const STATUS_TEST_ID = "entity-reference-registration-status";
const REQUEST_FAILURE_MESSAGE = "request failed";

vi.mock("@/lib/api/domains/task-change-registration-api", () => ({
  actTaskChangeRegistration,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, values?: Record<string, string>) =>
      Object.entries(values ?? {}).reduce(
        (value, [name, replacement]) => value.replace(`{{${name}}}`, replacement),
        key,
      ) + (values?.number ? ` ${values.number}` : ""),
  }),
}));

afterEach(() => {
  cleanup();
  actTaskChangeRegistration.mockReset();
});

function reference(overrides: Partial<EntityReference> = {}): EntityReference {
  return {
    version: 1,
    ref: "mention:v1:github:pull_request:github.com%2Facme%2Fapp:12",
    provider: "github",
    kind: "pull_request",
    id: "12",
    key: "acme/app#12",
    title: "Add authentication",
    url: "https://github.com/acme/app/pull/12",
    scope: "github.com/acme/app",
    association_mode: "link_on_send",
    ...overrides,
  };
}

describe("EntityReferenceChip registration status", () => {
  it("announces a pending link politely", () => {
    render(
      <EntityReferenceChip
        reference={reference({ registration_status: "pending" })}
        taskId={TASK_ID}
      />,
    );

    const status = screen.getByTestId(STATUS_TEST_ID);
    expect(status.textContent).toBe("task:referenceRegistrationPending");
    expect(status.getAttribute("aria-live")).toBe("polite");
  });

  it("shows the provider-specific linked change request number", () => {
    render(<EntityReferenceChip reference={reference({ registration_status: "linked" })} />);

    expect(screen.getByTestId(STATUS_TEST_ID).textContent).toBe(
      "task:referenceRegistrationLinkedPullRequest 12",
    );
  });
});

describe("EntityReferenceChip terminal and context-only states", () => {
  it("does not offer retry for a terminal identity failure", () => {
    render(
      <EntityReferenceChip
        reference={reference({
          selection_id: SELECTION_ID,
          registration_status: "failed",
          registration_error_code: "identity_unresolved",
        })}
        taskId={TASK_ID}
      />,
    );

    expect(screen.getByTestId(STATUS_TEST_ID).textContent).toBe("task:referenceRegistrationFailed");
    expect(screen.queryByTestId("entity-reference-registration-retry")).toBeNull();
  });

  it("labels context-only references without a registration retry", () => {
    render(
      <EntityReferenceChip
        reference={reference({ association_mode: "context_only", registration_status: undefined })}
        taskId={TASK_ID}
      />,
    );

    expect(screen.getByTestId(STATUS_TEST_ID).textContent).toBe("task:referenceContextOnly");
    expect(screen.queryByTestId("entity-reference-registration-retry")).toBeNull();
  });
});

describe("EntityReferenceChip retry actions", () => {
  it("retries the accepted selection without resending the message", async () => {
    actTaskChangeRegistration.mockResolvedValue({ accepted: true, state: "pending" });
    render(
      <EntityReferenceChip
        reference={reference({ selection_id: SELECTION_ID, registration_status: "uncertain" })}
        taskId={TASK_ID}
      />,
    );

    const retry = screen.getByRole("button", { name: "task:referenceRegistrationRetry" });
    expect(retry.className.split(/\s+/u)).toContain("min-h-7");
    expect(retry.className.split(/\s+/u)).toContain("max-md:min-h-11");
    expect(retry.className.split(/\s+/u)).toContain("cursor-pointer");
    expect(retry.className.split(/\s+/u)).toContain("[@media(pointer:coarse)]:min-h-11");
    fireEvent.click(retry);

    await waitFor(() =>
      expect(actTaskChangeRegistration).toHaveBeenCalledWith(TASK_ID, SELECTION_ID, "retry"),
    );
    expect(actTaskChangeRegistration).toHaveBeenCalledTimes(1);
  });

  it("reports a retry request failure to assistive technology", async () => {
    actTaskChangeRegistration.mockRejectedValue(new Error(REQUEST_FAILURE_MESSAGE));
    render(
      <EntityReferenceChip
        reference={reference({ selection_id: SELECTION_ID, registration_status: "uncertain" })}
        taskId={TASK_ID}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: "task:referenceRegistrationRetry" }));

    expect((await screen.findByRole("alert")).textContent).toBe(
      "task:referenceRegistrationRetryFailed",
    );
  });
});

describe("EntityReferenceChip relink actions", () => {
  it("shows unlinked state and restores only after an explicit Link again action", async () => {
    actTaskChangeRegistration.mockResolvedValue({ accepted: true, state: "pending" });
    render(
      <EntityReferenceChip
        reference={reference({ selection_id: SELECTION_ID, registration_status: "unlinked" })}
        taskId={TASK_ID}
      />,
    );

    expect(screen.getByTestId(STATUS_TEST_ID).textContent).toBe(
      "task:referenceRegistrationUnlinked",
    );
    const linkAgain = screen.getByTestId("entity-reference-registration-link-again");
    expect(linkAgain.className.split(/\s+/u)).toContain("min-h-7");
    expect(linkAgain.className.split(/\s+/u)).toContain("max-md:min-h-11");
    expect(linkAgain.className.split(/\s+/u)).toContain("cursor-pointer");
    expect(linkAgain.className.split(/\s+/u)).toContain("[@media(pointer:coarse)]:min-h-11");
    fireEvent.click(linkAgain);

    await waitFor(() =>
      expect(actTaskChangeRegistration).toHaveBeenCalledWith(TASK_ID, SELECTION_ID, "relink"),
    );
  });

  it("reports a failed Link again request accessibly", async () => {
    actTaskChangeRegistration.mockRejectedValue(new Error(REQUEST_FAILURE_MESSAGE));
    render(
      <EntityReferenceChip
        reference={reference({ selection_id: SELECTION_ID, registration_status: "unlinked" })}
        taskId={TASK_ID}
      />,
    );

    fireEvent.click(screen.getByTestId("entity-reference-registration-link-again"));

    expect((await screen.findByRole("alert")).textContent).toBe(
      "task:referenceRegistrationRelinkFailed",
    );
  });
});
