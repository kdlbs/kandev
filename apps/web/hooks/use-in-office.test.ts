import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";

let officeEnabled = false;
let pathname = "/";

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => officeEnabled,
}));
vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => pathname,
}));

import { useInOffice } from "./use-in-office";

function run(): boolean {
  return renderHook(() => useInOffice()).result.current;
}

describe("useInOffice", () => {
  afterEach(() => {
    officeEnabled = false;
    pathname = "/";
  });

  it("is false in the regular workspace even when the office feature is enabled", () => {
    officeEnabled = true;
    pathname = "/";
    expect(run()).toBe(false);
  });

  it("is false on a task route (/t/...) with office enabled", () => {
    officeEnabled = true;
    pathname = "/t/abc123";
    expect(run()).toBe(false);
  });

  it("is true on the office dashboard and office sub-routes", () => {
    officeEnabled = true;
    pathname = "/office";
    expect(run()).toBe(true);
    pathname = "/office/projects/p1";
    expect(run()).toBe(true);
    pathname = "/office/agents";
    expect(run()).toBe(true);
  });

  it("is false on /office routes when the office feature is disabled", () => {
    officeEnabled = false;
    pathname = "/office";
    expect(run()).toBe(false);
  });

  it("does not match a route that merely starts with the word office", () => {
    officeEnabled = true;
    pathname = "/officers"; // not an /office route
    expect(run()).toBe(false);
  });
});
