import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createDefaultKubernetesProfileConfig } from "./kubernetes-config";
import { KubernetesWorkloadCard } from "./kubernetes-workload-card";

const originalScrollHeight = Object.getOwnPropertyDescriptor(
  HTMLTextAreaElement.prototype,
  "scrollHeight",
);

beforeEach(() => {
  Object.defineProperty(HTMLTextAreaElement.prototype, "scrollHeight", {
    configurable: true,
    get() {
      return this.value.includes("long-image-reference") ? 480 : 96;
    },
  });
});

afterEach(() => {
  cleanup();
  if (originalScrollHeight) {
    Object.defineProperty(HTMLTextAreaElement.prototype, "scrollHeight", originalScrollHeight);
  }
});

describe("KubernetesWorkloadCard PodTemplate sizing", () => {
  it("grows and shrinks with controlled YAML while keeping horizontal-only overflow", () => {
    const baseline = createDefaultKubernetesProfileConfig();
    const shortForm = { ...baseline, podTemplateYaml: "kind: PodTemplate" };
    const { rerender } = render(
      <KubernetesWorkloadCard form={shortForm} baseline={baseline} onChange={vi.fn()} />,
    );
    const textarea = screen.getByTestId("kubernetes-pod-template") as HTMLTextAreaElement;

    expect(textarea.style.height).toBe("96px");
    expect(textarea.className).toContain("overflow-y-hidden");
    expect(textarea.className).toContain("overflow-x-auto");
    expect(textarea.className).toContain("resize-none");

    rerender(
      <KubernetesWorkloadCard
        form={{ ...shortForm, podTemplateYaml: "image: long-image-reference\n".repeat(20) }}
        baseline={baseline}
        onChange={vi.fn()}
      />,
    );
    expect(textarea.style.height).toBe("480px");

    rerender(<KubernetesWorkloadCard form={shortForm} baseline={baseline} onChange={vi.fn()} />);
    expect(textarea.style.height).toBe("96px");
  });
});
