import { describe, it, expect } from "vitest";
import {
  USE_DEFAULT,
  orgSelectItems,
  projectSelectItems,
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

describe("orgSelectItems", () => {
  it("prepends a Use default option when a default org is configured", () => {
    const items = orgSelectItems(["acme", "globex"], "", "acme");
    expect(items[0]).toEqual({ id: USE_DEFAULT, label: "Use default (acme)" });
    expect(items.map((i) => i.id)).toEqual([USE_DEFAULT, "acme", "globex"]);
  });

  it("omits the Use default option when no default is configured", () => {
    const items = orgSelectItems(["acme"], "", "");
    expect(items.some((i) => i.id === USE_DEFAULT)).toBe(false);
    expect(items.map((i) => i.id)).toEqual(["acme"]);
  });

  it("keeps the current value even if the token can no longer see it", () => {
    const items = orgSelectItems(["acme"], "legacy-org", "");
    expect(items.map((i) => i.id)).toEqual(["legacy-org", "acme"]);
  });

  it("does not duplicate the current value when it is also in the list", () => {
    const items = orgSelectItems(["acme", "globex"], "acme", "");
    expect(items.map((i) => i.id)).toEqual(["acme", "globex"]);
  });
});

describe("projectSelectItems", () => {
  const projects = [proj("frontend", "Frontend"), proj("api", "API")];

  it("labels projects as 'name (slug)'", () => {
    const items = projectSelectItems(projects, "", "");
    expect(items).toEqual([
      { id: "frontend", label: "Frontend (frontend)" },
      { id: "api", label: "API (api)" },
    ]);
  });

  it("offers Use default only when the default project is in the visible list", () => {
    const inOrg = projectSelectItems(projects, "", "frontend");
    expect(inOrg[0]).toEqual({ id: USE_DEFAULT, label: "Use default (frontend)" });

    const outOfOrg = projectSelectItems(projects, "", "billing");
    expect(outOfOrg.some((i) => i.id === USE_DEFAULT)).toBe(false);
  });

  it("keeps a current project not present in the visible list", () => {
    const items = projectSelectItems(projects, "archived", "");
    expect(items.map((i) => i.id)).toContain("archived");
  });
});

describe("buildFilterPayload", () => {
  it("trims the org slug and drops an empty project slug", () => {
    const form = { ...makeEmptyForm("ws-1"), orgSlug: "  acme  ", projectSlug: "" };
    const filter = buildFilterPayload(form);
    expect(filter.orgSlug).toBe("acme");
    expect(filter.projectSlug).toBeUndefined();
  });
});
