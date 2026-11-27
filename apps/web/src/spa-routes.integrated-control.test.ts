import { describe, expect, it } from "vitest";
import { resolveSpaRoute } from "./spa-routes";
import { CONTROL_VIEWS } from "@/lib/navigation/integrated-control";

describe("Integrated control route admission", () => {
  it.each(["/control", ...CONTROL_VIEWS.map((view) => `/control/${view}`)])(
    "requires explicit feature admission for %s",
    (path) => {
      expect(resolveSpaRoute(path, new URLSearchParams()).kind).not.toBe("integratedControl");
      expect(
        resolveSpaRoute(path, new URLSearchParams(), { integratedControlEnabled: false }).kind,
      ).not.toBe("integratedControl");
      expect(
        resolveSpaRoute(path, new URLSearchParams(), { integratedControlEnabled: true }),
      ).toEqual({ kind: "integratedControl" });
    },
  );
  it.each(["/controls", "/tasks", "/workspaces/ws/coordinator"])(
    "retains ordinary route ownership for %s",
    (path) => {
      const options = { coordinatorEnabled: true };
      expect(
        resolveSpaRoute(path, new URLSearchParams(), {
          ...options,
          integratedControlEnabled: true,
        }),
      ).toEqual(resolveSpaRoute(path, new URLSearchParams(), options));
    },
  );
});
