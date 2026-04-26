import { describe, expect, it } from "vitest";
import { DEFAULT_FILTERS, filtersToJql, filtersEqual } from "./filter-model";

describe("filtersToJql", () => {
  it("produces default query for default state", () => {
    expect(filtersToJql(DEFAULT_FILTERS)).toBe("assignee = currentUser() ORDER BY updated DESC");
  });

  it("omits where clause when everything is open", () => {
    expect(filtersToJql({ ...DEFAULT_FILTERS, assignee: "anyone" })).toBe("ORDER BY updated DESC");
  });

  it("quotes project keys", () => {
    const jql = filtersToJql({ ...DEFAULT_FILTERS, projectKeys: ["SC", "ECOM"] });
    expect(jql).toBe(
      `project in ("SC", "ECOM") AND assignee = currentUser() ORDER BY updated DESC`,
    );
  });

  it("maps status categories to Jira labels", () => {
    const jql = filtersToJql({
      ...DEFAULT_FILTERS,
      statusCategories: ["new", "indeterminate"],
      assignee: "anyone",
    });
    expect(jql).toBe(`statusCategory in ("To Do", "In Progress") ORDER BY updated DESC`);
  });

  it("detects ticket key in search text", () => {
    const jql = filtersToJql({ ...DEFAULT_FILTERS, searchText: "SC-123", assignee: "anyone" });
    expect(jql).toBe(`key = "SC-123" ORDER BY updated DESC`);
  });

  it("uses text search for free-form query", () => {
    const jql = filtersToJql({
      ...DEFAULT_FILTERS,
      searchText: "login bug",
      assignee: "anyone",
    });
    expect(jql).toBe(`text ~ "login bug" ORDER BY updated DESC`);
  });

  it("escapes quotes in search text", () => {
    const jql = filtersToJql({
      ...DEFAULT_FILTERS,
      searchText: `"bad" query`,
      assignee: "anyone",
    });
    expect(jql).toBe(`text ~ "\\"bad\\" query" ORDER BY updated DESC`);
  });

  it("renders unassigned filter", () => {
    expect(filtersToJql({ ...DEFAULT_FILTERS, assignee: "unassigned" })).toBe(
      "assignee is EMPTY ORDER BY updated DESC",
    );
  });

  it("switches sort clause", () => {
    expect(filtersToJql({ ...DEFAULT_FILTERS, sort: "priority", assignee: "anyone" })).toBe(
      "ORDER BY priority DESC, updated DESC",
    );
    expect(filtersToJql({ ...DEFAULT_FILTERS, sort: "created", assignee: "anyone" })).toBe(
      "ORDER BY created DESC",
    );
  });
});

describe("filtersEqual", () => {
  it("returns true for deep-equal state", () => {
    expect(filtersEqual(DEFAULT_FILTERS, { ...DEFAULT_FILTERS })).toBe(true);
  });

  it("returns false when arrays differ", () => {
    expect(filtersEqual(DEFAULT_FILTERS, { ...DEFAULT_FILTERS, projectKeys: ["X"] })).toBe(false);
  });
});
