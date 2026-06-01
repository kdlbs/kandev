import { describe, it, expect } from "vitest";
import {
  USE_DEFAULT,
  orgSelectItems,
  projectSelectItems,
  resolveSlugSelection,
  buildFilterPayload,
  makeEmptyForm,
} from "./sentry-issue-watch-form";
import type { SentryProject } from "@/lib/types/sentry";

const proj = (slug: string, name: string, orgSlug = "acme"): SentryProject => ({
  id: slug,
  slug,
  name,
  orgSlug,
});

describe("resolveSlugSelection", () => {
  it("collapses the Use default sentinel to an empty string", () => {
    expect(resolveSlugSelection(USE_DEFAULT)).toBe("");
  });

  it("passes a concrete slug through unchanged", () => {
    expect(resolveSlugSelection("acme")).toBe("acme");
  });
});

describe("orgSelectItems", () => {
  it("always leads with a Use default option, labelled with the configured default", () => {
    const items = orgSelectItems(["acme", "globex"], "", "acme");
    expect(items[0]).toEqual({ id: USE_DEFAULT, label: "Use default (acme)" });
    expect(items.map((i) => i.id)).toEqual([USE_DEFAULT, "acme", "globex"]);
  });

  it("keeps the Use default option even when no default is configured", () => {
    const items = orgSelectItems(["acme"], "", "");
    expect(items[0]).toEqual({ id: USE_DEFAULT, label: "Use default" });
    expect(items.map((i) => i.id)).toEqual([USE_DEFAULT, "acme"]);
  });

  it("keeps the current value even if the token can no longer see it", () => {
    const items = orgSelectItems(["acme"], "legacy-org", "");
    expect(items.map((i) => i.id)).toEqual([USE_DEFAULT, "legacy-org", "acme"]);
  });

  it("does not duplicate the current value when it is also in the list", () => {
    const items = orgSelectItems(["acme", "globex"], "acme", "");
    expect(items.map((i) => i.id)).toEqual([USE_DEFAULT, "acme", "globex"]);
  });
});

describe("projectSelectItems", () => {
  const projects = [proj("frontend", "Frontend"), proj("api", "API")];

  it("always leads with Use default, then labels projects as 'name (slug)'", () => {
    const items = projectSelectItems(projects, "", "frontend");
    expect(items).toEqual([
      { id: USE_DEFAULT, label: "Use default (frontend)" },
      { id: "frontend", label: "Frontend (frontend)" },
      { id: "api", label: "API (api)" },
    ]);
  });

  it("keeps the current project even if not in the visible list", () => {
    const items = projectSelectItems(projects, "archived", "");
    expect(items[0].id).toBe(USE_DEFAULT);
    expect(items.map((i) => i.id)).toContain("archived");
  });
});

describe("buildFilterPayload", () => {
  it("emits an empty org slug for 'use default' and drops an empty project slug", () => {
    const form = { ...makeEmptyForm("ws-1"), orgSlug: "", projectSlug: "" };
    const filter = buildFilterPayload(form);
    expect(filter.orgSlug).toBe("");
    expect(filter.projectSlug).toBeUndefined();
  });

  it("trims a concrete org slug", () => {
    const form = { ...makeEmptyForm("ws-1"), orgSlug: "  acme  ", projectSlug: "web" };
    const filter = buildFilterPayload(form);
    expect(filter.orgSlug).toBe("acme");
    expect(filter.projectSlug).toBe("web");
  });
});
