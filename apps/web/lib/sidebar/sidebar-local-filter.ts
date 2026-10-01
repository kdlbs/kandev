import type { SidebarTaskQuery } from "@/lib/types/http";

export function sqliteLower(value: string): string {
  return value.replace(/[A-Z]/g, (letter) => letter.toLowerCase());
}

export function matchesSidebarClause(
  actual: string,
  clause: SidebarTaskQuery["filters"][number],
): boolean {
  const values = Array.isArray(clause.value) ? clause.value.map(String) : [String(clause.value)];
  switch (clause.op) {
    case "is":
      return actual === values[0];
    case "is_not":
      return actual !== values[0];
    case "in":
      return values.includes(actual);
    case "not_in":
      return !values.includes(actual);
    case "matches":
      return sqliteLower(actual).includes(sqliteLower(values[0]));
    case "not_matches":
      return values[0] !== "" && !sqliteLower(actual).includes(sqliteLower(values[0]));
    default:
      return false;
  }
}

export function sidebarCanIncludeArchives(filters: SidebarTaskQuery["filters"]): boolean {
  const clauses = filters.filter((filter) => filter.dimension === "archived");
  return clauses.length > 0 && clauses.every((clause) => matchesSidebarClause("true", clause));
}
