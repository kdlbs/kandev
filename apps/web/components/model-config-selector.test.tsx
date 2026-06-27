import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { ModelConfigSelector } from "@/components/model-config-selector";

afterEach(() => {
  cleanup();
});

describe("ModelConfigSelector", () => {
  it("passes custom trigger classes to the button", () => {
    render(
      <ModelConfigSelector
        modelOptions={[{ id: "gpt-5.5", name: "GPT-5.5" }]}
        currentModel="gpt-5.5"
        onModelChange={() => {}}
        triggerClassName="max-w-[56vw]"
      />,
    );

    expect(screen.getByRole("button", { name: "Model settings" }).className).toContain(
      "max-w-[56vw]",
    );
  });
});
