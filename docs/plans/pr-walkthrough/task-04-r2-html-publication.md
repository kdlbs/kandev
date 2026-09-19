---
id: "04-r2-html-publication"
title: "Publish walkthrough HTML to Cloudflare R2"
status: done
wave: 3
depends_on: ["03-workflow-artifact-generation"]
plan: "plan.md"
spec: "../../specs/ui/requirements/pr-walkthrough.md"
---

# Task 04: Publish walkthrough HTML to Cloudflare R2

Add the dependent publication job that downloads the successful generation
artifact and uploads only the rendered HTML to the already-configured
Cloudflare R2 bucket. Keep the R2 credentials isolated to this job and use the
custom domain as the externally visible URL.

- **Acceptance:** The publication job runs only after successful walkthrough
  generation and only for an authorized, non-draft same-repository pull
  request. A label-triggered rerun publishes the current head without enabling
  `synchronize` generation.
- **Acceptance:** The job downloads the generation artifact but does not check
  out or execute PR-owned files and does not expose R2 credentials to the
  generation job.
- **Acceptance:** The job uploads only the HTML file through the R2
  S3-compatible API using a bucket-scoped Object Read & Write token. It does
  not use the existing Pages `CLOUDFLARE_API_TOKEN`.
- **Acceptance:** The object key is
  `pr/<pull-request-number>/<short-head-sha>.html`, where `short-head-sha` is
  the first 12 lowercase hexadecimal characters of the exact head SHA. It has
  HTML content type and a short cache policy suitable for label-triggered
  reruns.
- **Acceptance:** The job verifies the object with an R2 metadata request and
  verifies the public URL
  `https://walkthrough.kandev.ai/pr/<pull-request-number>/<short-head-sha>.html`
  with an HTTPS GET before reporting publication success.
- **Acceptance:** The job writes the public URL to the workflow job summary
  and exports it to the dependent trusted link job, but receives no GitHub
  write permission and does not publish the JSON artifact.
- **Acceptance:** The workflow contract test covers the dependency, event
  gate, artifact download, S3 endpoint, bucket/key construction, secret and
  variable names, content metadata, public URL check, and the absence of
  generation-job credentials.

- **Verification:**

  ```text
  python3 .github/scripts/pr-walkthrough-workflow-contract_test.py
  python3 .github/scripts/lint-action-pinning_test.py
  python3 .github/scripts/lint-action-pinning.py
  zizmor .github/workflows/pr-walkthrough.yml
  git diff --check
  ```

- **Files likely touched:** `.github/workflows/pr-walkthrough.yml`,
  `.github/scripts/pr-walkthrough-workflow-contract_test.py`, and possibly
  `.github/workflows/lint-action-pinning.yml` if the contract test wiring is
  added in this task.
- **Dependencies:** Task 03 and the operator-provisioned R2 bucket,
  `walkthrough.kandev.ai` custom domain, `pr/` lifecycle rule, GitHub Actions
  secrets, and GitHub Actions variables.
- **Parallelism:** sequential, because this job consumes task 03's artifact and
  edits the shared OpenCode workflow.
- **Inputs:** The spec's R2 publication contract and retention semantics; the
  existing Cloudflare Pages workflow's pinned-action conventions; the
  bucket-scoped R2 S3 credentials configured in GitHub.
- **Output contract:** Report changed workflow and contract-test files,
  security/static-check results, the generated object URL from any configured
  CI dry run, and any unavailable external Cloudflare dependency. Keep the
  task pending until implementation begins.

## Results

Added the dependent artifact-only publication job. It uploads only the HTML
file to the configured R2 S3 endpoint using the bucket-scoped credentials,
checks object metadata, performs a bounded-retry public HTTPS GET and byte
comparison, and writes and exports the custom URL without receiving GitHub
write permission.

Verification: workflow contract tests passed; the publication job has no
checkout step, and R2 secrets are scoped only to the upload step. Operator
provisioning is complete; a live upload remains pending the first eligible PR
workflow run.

Follow-up remediation: added `Cache-Control: no-transform` to the R2 upload so
Cloudflare cannot rewrite the HTML before the public byte comparison. Updated
the workflow contract regression to require the cache directive.

Follow-up verification: `python3 .github/scripts/pr-walkthrough-workflow-contract_test.py`
(22 tests passed), `python3 .github/scripts/lint-action-pinning_test.py` (9 tests
passed), `python3 .github/scripts/lint-action-pinning.py` (20 workflows passed),
and `git diff --check` passed. `zizmor` 1.25.2 retained the existing
`dangerous-triggers` finding for the intentional `pull_request_target` workflow;
the finding is outside this diff. `actionlint` was unavailable locally.
