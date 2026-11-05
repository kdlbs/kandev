import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { parseExecutorProfileSchema } from "@/lib/plugins/executor-profile-schema";
import { PluginExecutorProfileFields } from "./plugin-executor-profile-fields";

afterEach(() => cleanup());

describe("PluginExecutorProfileFields schema defaults", () => {
  it("shows a schema default beside its field and never for a secret", () => {
    const { fields } = parseExecutorProfileSchema({
      type: "object",
      properties: {
        tier: { type: "string", title: "Tier", default: "standard", maxLength: 8 },
        credential: { type: "string", title: "Credential", secret: true },
      },
    });
    render(
      <PluginExecutorProfileFields
        fields={fields}
        values={{ tier: "", credential: "" }}
        onValueChange={vi.fn()}
      />,
    );

    expect(screen.getByTestId("executor-profile-default-tier")).not.toBeNull();
    expect(screen.queryByTestId("executor-profile-default-credential")).toBeNull();
    expect((screen.getByLabelText(/Tier/) as HTMLInputElement).maxLength).toBe(8);
  });
});
