import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { useState } from "react";
import { ProfileEnvVarsEditor } from "@/components/settings/agent-profile-page";
import type { ProfileEnvVar } from "@/lib/types/http";

afterEach(cleanup);

function EnvVarsHarness({ initialEnvVars = [] }: { initialEnvVars?: ProfileEnvVar[] }) {
  const [envVars, setEnvVars] = useState<ProfileEnvVar[]>(initialEnvVars);
  const [changeCount, setChangeCount] = useState(0);

  return (
    <>
      <ProfileEnvVarsEditor
        envVars={envVars}
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

  it("emits a row edit once without looping through parent state", async () => {
    render(<EnvVarsHarness />);

    fireEvent.click(screen.getByRole("button", { name: /add/i }));
    fireEvent.change(screen.getByPlaceholderText("KEY"), { target: { value: "FOO" } });

    await waitFor(() => expect(screen.getByTestId("change-count").textContent).toBe("1"));
  });
});
