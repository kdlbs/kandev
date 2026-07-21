export const missingGitHealth = {
  healthy: false,
  issues: [
    {
      id: "git_executable_missing",
      category: "system_requirements",
      title: "Git executable is required",
      message: "Install Git and ensure the git executable is available on PATH.",
      severity: "error",
      fix_url: "/settings/system/status",
      fix_label: "View system status",
    },
  ],
  checks: [],
};
