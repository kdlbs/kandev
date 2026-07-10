import { describe, it, expect } from "vitest";
import {
  orgSelectItems,
  projectSelectItems,
  maxInflightTasksString,
  parseMaxInflightTasks,
  buildFilterPayload,
  isWatchFormReady,
  makeEmptyForm,
} from "./sentry-issue-watch-form";
import type { SentryProject } from "@/lib/types/sentry";

const proj = (slug: string, name: string, orgSlug = "acme"): SentryProject => ({
  id: slug,
  slug,
  name,
  orgSlug,
});

describe("orgSelectItems", () => {
  it("lists the orgs the token can see", () => {
    const items = orgSelectItems(["acme", "globex"], "");
    expect(items.map((i) => i.id)).toEqual(["acme", "globex"]);
  });

  it("keeps the current value even if the token can no longer see it", () => {
    const items = orgSelectItems(["acme"], "legacy-org");
    expect(items.map((i) => i.id)).toEqual(["legacy-org", "acme"]);
  });

  it("does not duplicate the current value when it is also in the list", () => {
    const items = orgSelectItems(["acme", "globex"], "acme");
    expect(items.map((i) => i.id)).toEqual(["acme", "globex"]);
  });
});

describe("projectSelectItems", () => {
  const projects = [proj("frontend", "Frontend"), proj("api", "API")];

  it("labels projects as 'name (slug)'", () => {
    const items = projectSelectItems(projects, "");
    expect(items).toEqual([
      { id: "frontend", label: "Frontend (frontend)" },
      { id: "api", label: "API (api)" },
    ]);
  });

  it("keeps the current project even if not in the visible list", () => {
    const items = projectSelectItems(projects, "archived");
    expect(items.map((i) => i.id)).toContain("archived");
  });
});

describe("maxInflightTasksString", () => {
  it("renders nil / non-positive caps as blank (uncapped)", () => {
    expect(maxInflightTasksString(null)).toBe("");
    expect(maxInflightTasksString(undefined)).toBe("");
    expect(maxInflightTasksString(0)).toBe("");
    expect(maxInflightTasksString(-3)).toBe("");
  });

  it("renders a positive cap as its string form", () => {
    expect(maxInflightTasksString(5)).toBe("5");
  });
});

describe("parseMaxInflightTasks", () => {
  it("maps blank to null (uncapped)", () => {
    expect(parseMaxInflightTasks("")).toBeNull();
    expect(parseMaxInflightTasks("   ")).toBeNull();
  });

  it("parses a positive whole number", () => {
    expect(parseMaxInflightTasks("5")).toBe(5);
    expect(parseMaxInflightTasks(" 12 ")).toBe(12);
  });

  it("flags non-positive or non-integer input as invalid", () => {
    expect(parseMaxInflightTasks("0")).toBe("invalid");
    expect(parseMaxInflightTasks("-1")).toBe("invalid");
    expect(parseMaxInflightTasks("1.5")).toBe("invalid");
    expect(parseMaxInflightTasks("abc")).toBe("invalid");
  });
});

describe("buildFilterPayload", () => {
  it("trims the org slug and drops an empty project slug", () => {
    const form = { ...makeEmptyForm("ws-1"), orgSlug: "  acme  ", projectSlug: "" };
    const filter = buildFilterPayload(form);
    expect(filter.orgSlug).toBe("acme");
    expect(filter.projectSlug).toBeUndefined();
  });

  it("keeps a concrete project slug", () => {
    const form = { ...makeEmptyForm("ws-1"), orgSlug: "acme", projectSlug: "web" };
    const filter = buildFilterPayload(form);
    expect(filter.orgSlug).toBe("acme");
    expect(filter.projectSlug).toBe("web");
  });
});

describe("isWatchFormReady", () => {
  it("allows a legacy unbound watch to update its mutable fields", () => {
    const legacyUnbound = {
      ...makeEmptyForm("ws-1"),
      orgSlug: "acme",
      projectSlug: "web",
      workflowId: "workflow-1",
      workflowStepId: "step-1",
      sentryInstanceId: "",
    };

    expect(isWatchFormReady(legacyUnbound, { requiresInstance: false })).toBe(true);
  });
});
