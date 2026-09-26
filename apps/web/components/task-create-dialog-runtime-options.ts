import { useState } from "react";

export function useTaskCreateDialogRuntimeOptions() {
  // No-repo mode can point the agent at an existing host folder. An empty path uses a scratch workspace.
  const [noRepository, setNoRepository] = useState(false);
  const [preferLocalExecutor, setPreferLocalExecutor] = useState(false);
  const [workspacePath, setWorkspacePath] = useState("");
  const [autopilot, setAutopilot] = useState(false);
  const [autoCreatePR, setAutoCreatePR] = useState(false);
  return {
    noRepository,
    setNoRepository,
    preferLocalExecutor,
    setPreferLocalExecutor,
    workspacePath,
    setWorkspacePath,
    autopilot,
    setAutopilot,
    autoCreatePR,
    setAutoCreatePR,
  };
}
