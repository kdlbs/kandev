import { describe, it, expect } from "vitest";
import {
  orgSelectItems,
  projectMultiSelectItems,
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

describe("projectMultiSelectItems", () => {
  const projects = [proj("frontend", "Frontend"), proj("api", "API")];

  it("labels projects as 'name (slug)'", () => {
    const items = projectMultiSelectItems(projects, []);
    expect(items).toEqual([
      { id: "frontend", label: "Frontend (frontend)" },
      { id: "api", label: "API (api)" },
    ]);
  });

  it("keeps every currently-selected project even if not in the visible list", () => {
    const items = projectMultiSelectItems(projects, ["archived", "frontend"]);
    expect(items.map((i) => i.id)).toEqual(["frontend", "api", "archived"]);
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
  it("trims the org slug and drops an empty project selection", () => {
    const form = { ...makeEmptyForm("ws-1"), orgSlug: "  acme  ", projectSlugs: [] };
    const filter = buildFilterPayload(form);
    expect(filter.orgSlug).toBe("acme");
    expect(filter.projectSlugs).toBeUndefined();
  });

  it("keeps a single selected project", () => {
    const form = { ...makeEmptyForm("ws-1"), orgSlug: "acme", projectSlugs: ["web"] };
    const filter = buildFilterPayload(form);
    expect(filter.orgSlug).toBe("acme");
    expect(filter.projectSlugs).toEqual(["web"]);
  });

  it("keeps every selected project when several are chosen", () => {
    const form = { ...makeEmptyForm("ws-1"), orgSlug: "acme", projectSlugs: ["web", "api"] };
    const filter = buildFilterPayload(form);
    expect(filter.projectSlugs).toEqual(["web", "api"]);
  });
});

describe("isWatchFormReady", () => {
  it("allows a legacy unbound watch to update its mutable fields", () => {
    const legacyUnbound = {
      ...makeEmptyForm("ws-1"),
      orgSlug: "acme",
      projectSlugs: ["web"],
      workflowId: "workflow-1",
      workflowStepId: "step-1",
      sentryInstanceId: "",
    };

    expect(isWatchFormReady(legacyUnbound, { requiresInstance: false })).toBe(true);
  });

  it("rejects a form with no project selected", () => {
    const form = {
      ...makeEmptyForm("ws-1"),
      orgSlug: "acme",
      projectSlugs: [],
      workflowId: "workflow-1",
      workflowStepId: "step-1",
    };

    expect(isWatchFormReady(form)).toBe(false);
  });

  it("accepts a form with several projects selected", () => {
    const form = {
      ...makeEmptyForm("ws-1"),
      sentryInstanceId: "instance-a",
      orgSlug: "acme",
      projectSlugs: ["web", "api"],
      workflowId: "workflow-1",
      workflowStepId: "step-1",
    };

    expect(isWatchFormReady(form)).toBe(true);
  });
});
