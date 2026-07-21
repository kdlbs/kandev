import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { useState } from "react";
import {
  preserveNewerProfileDraft,
  ProfileEnvVarsEditor,
} from "@/components/settings/agent-profile-page";
import type { AgentProfile, ProfileEnvVar } from "@/lib/types/http";

afterEach(cleanup);
const DIRTY_ATTRIBUTE = "data-settings-dirty";

function EnvVarsHarness({ initialEnvVars = [] }: { initialEnvVars?: ProfileEnvVar[] }) {
  const [envVars, setEnvVars] = useState<ProfileEnvVar[]>(initialEnvVars);
  const [changeCount, setChangeCount] = useState(0);

  return (
    <>
      <ProfileEnvVarsEditor
        envVars={envVars}
        baselineEnvVars={initialEnvVars}
        secrets={[]}
        onChange={(nextEnvVars) => {
          setChangeCount((count) => count + 1);
          setEnvVars(nextEnvVars);
        }}
      />
      <span data-testid="change-count">{changeCount}</span>
    </>
  );
}

describe("ProfileEnvVarsEditor", () => {
  it("does not emit unchanged env vars on mount", () => {
    render(<EnvVarsHarness initialEnvVars={[{ key: "FOO", value: "bar" }]} />);

    expect(screen.getByTestId("change-count").textContent).toBe("0");
  });

  it("emits exactly one change when adding a row via the add form", async () => {
    render(<EnvVarsHarness />);

    fireEvent.change(screen.getByTestId("env-var-new-key-input"), { target: { value: "FOO" } });
    fireEvent.click(screen.getByTestId("env-var-add-button"));

    await waitFor(() => expect(screen.getByTestId("change-count").textContent).toBe("1"));
    expect(screen.getByTestId("env-vars-card").getAttribute(DIRTY_ATTRIBUTE)).toBe("true");
    expect(screen.getByTestId("env-var-row-0").getAttribute(DIRTY_ATTRIBUTE)).toBe("true");
  });

  it("marks only the changed saved row and its owning card dirty", () => {
    render(<EnvVarsHarness initialEnvVars={[{ key: "FOO", value: "bar" }]} />);

    expect(screen.getByTestId("env-vars-card").getAttribute(DIRTY_ATTRIBUTE)).toBe("false");
    fireEvent.change(screen.getByDisplayValue("bar"), { target: { value: "baz" } });

    expect(screen.getByTestId("env-vars-card").getAttribute(DIRTY_ATTRIBUTE)).toBe("true");
    expect(screen.getByTestId("env-var-row-0").getAttribute(DIRTY_ATTRIBUTE)).toBe("true");
  });
});

describe("preserveNewerProfileDraft", () => {
  it("keeps a profile edit made while save is in flight", () => {
    const submitted = profile("submitted");
    const current = { ...submitted, name: "newer" };
    const saved = { ...submitted, updatedAt: "saved" };

    expect(preserveNewerProfileDraft(current, submitted, saved)).toBe(current);
    expect(preserveNewerProfileDraft(submitted, submitted, saved)).toBe(saved);
  });
});

function profile(name: string): AgentProfile {
  return {
    id: "profile-1" as AgentProfile["id"],
    agentId: "agent-1",
    name,
    agentDisplayName: "Agent",
    model: "model",
    allowIndexing: false,
    autoApprove: false,
    cliFlags: [],
    cliPassthrough: false,
    createdAt: "",
    updatedAt: "",
  };
}
