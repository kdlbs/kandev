import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/client";
import { i18n } from "@/lib/i18n";
import { secretDeleteErrorMessage, secretReferencesFromError } from "./secret-delete-error";

const t = i18n.getFixedT("en");
const PRIVATE_DETAILS = "private details";

describe("secretDeleteErrorMessage", () => {
  it("localizes known reference kinds and preserves hidden metadata", () => {
    const error = new ApiError(PRIVATE_DETAILS, 409, {
      code: "secret_in_use",
      references: [
        { kind: "executor_profile", name: "Local", key: "TOKEN" },
        { kind: "repository", name: "App", key: "APP_TOKEN" },
        { kind: "repository" },
      ],
    });
    expect(secretDeleteErrorMessage(error, t)).toBe(
      'This secret is in use by: Executor profile "Local" (TOKEN), Repository "App" (APP_TOKEN), A resource you cannot access. Remove or replace these references before deleting it.',
    );
  });

  it.each([null, {}, { references: [] }, { code: "different_conflict" }])(
    "does not expose raw error text for an unknown response %j",
    (body) => {
      expect(secretDeleteErrorMessage(new ApiError(PRIVATE_DETAILS, 409, body), t)).toBe(
        "Couldn't delete secret",
      );
    },
  );

  it("handles missing and malformed references without losing the conflict reason", () => {
    for (const references of [
      undefined,
      null,
      "bad",
      [null, {}, { kind: "unknown", name: "private", key: "x" }],
    ]) {
      const error = new ApiError(PRIVATE_DETAILS, 409, { code: "secret_in_use", references });
      expect(secretDeleteErrorMessage(error, t)).toBe(
        "This secret is in use. Remove or replace its references before deleting it.",
      );
    }
  });
});

describe("secretReferencesFromError", () => {
  it("returns structured references from an in-use delete race", () => {
    const references = [{ kind: "repository", id: "repo-1", name: "App", key: "DEPLOY_TOKEN" }];

    expect(
      secretReferencesFromError(
        new ApiError(PRIVATE_DETAILS, 409, { code: "secret_in_use", references }),
      ),
    ).toEqual(references);
  });

  it("rejects malformed and unrelated errors", () => {
    expect(secretReferencesFromError(new Error(PRIVATE_DETAILS))).toBeNull();
    expect(
      secretReferencesFromError(
        new ApiError(PRIVATE_DETAILS, 409, { code: "secret_in_use", references: "bad" }),
      ),
    ).toBeNull();
  });
});
