import { describe, expect, it } from "vitest";
import { homeDestinationHref } from "./core-destinations";
import { workspaceHomeHref } from "./workspace-home";
import type { StartupPage } from "@/lib/types/http-user-settings";

// @covers AC-UI-TASK-LISTING-DISPLAY-PREFERENCES-003.4, 003.6, 003.7
describe("Home destination", () => {
  it.each<StartupPage>(["task_overview", "last_task", "threads"])(
    "preserves Office and no-workspace fallback for %s",
    (startupPage) => {
      expect(workspaceHomeHref({ id: "office/1", office_workflow_id: "wf" }, startupPage)).toBe(
        "/office?workspaceId=office%2F1",
      );
      expect(workspaceHomeHref(undefined, startupPage)).toBe("/?home=overview");
    },
  );

  it.each<StartupPage>(["task_overview", "last_task", "threads"])(
    "shares workspace-aware policy with the manifest for %s",
    (startupPage) => {
      const expected =
        startupPage === "threads"
          ? "/threads?workspace=ws%2F2%26x"
          : "/?home=overview&workspaceId=ws%2F2%26x";
      expect(workspaceHomeHref({ id: "ws/2&x" }, startupPage)).toBe(expected);
      expect(homeDestinationHref({ workspaceId: "ws/2&x", inOffice: false, startupPage })).toBe(
        expected,
      );
      expect(homeDestinationHref({ workspaceId: "ws/2&x", inOffice: true, startupPage })).toBe(
        "/office?workspaceId=ws%2F2%26x",
      );
    },
  );
});
