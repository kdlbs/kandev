import { describe, expect, it } from "vitest";

import { serializeSettingsRevision } from "./settings-save-revision";

describe("serializeSettingsRevision", () => {
  it("ignores nested object key order", () => {
    const submitted = {
      name: "default",
      config: {
        git_user_name: "E2E Test",
        git_user_email: "e2e@test.local",
        dockerfile: "FROM node:22-slim",
        image_tag: "kandev/multi-agent:latest",
      },
    };
    const returned = {
      config: {
        dockerfile: "FROM node:22-slim",
        git_user_email: "e2e@test.local",
        git_user_name: "E2E Test",
        image_tag: "kandev/multi-agent:latest",
      },
      name: "default",
    };

    expect(serializeSettingsRevision(submitted)).toBe(serializeSettingsRevision(returned));
  });

  it("preserves array order", () => {
    expect(serializeSettingsRevision({ values: ["first", "second"] })).not.toBe(
      serializeSettingsRevision({ values: ["second", "first"] }),
    );
  });
});
