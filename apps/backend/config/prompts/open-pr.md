Please create and open a Pull Request for the current branch using the GitHub CLI (gh).

**PR Creation Steps:**
1. **Analyze the branch:**
   - Review all commits on this branch
   - Identify the changes made
   - Understand the purpose and scope of the work

2. **Check for PR template:**
   - Look for a PR template in .github/pull_request_template.md or .github/PULL_REQUEST_TEMPLATE.md
   - If a template exists, use it as the structure for the PR description
   - If no template exists, use the default format below

3. **Generate PR description:**
   Create a comprehensive PR description that includes:
   - **Title:** Conventional Commit summary (`type: description` or `type(scope): description`), 50-72 characters. Types: feat, fix, docs, refactor, perf, chore, ci, test
   - **Overview:** Brief description of what this PR does
   - **Changes:** Detailed list of changes made
   - **Motivation:** Why these changes are needed
   - **Testing:** How the changes were tested
   - **Screenshots/Examples:** If applicable (for UI changes)
   - **Breaking Changes:** Any breaking changes (if applicable)
   - **Related Issues:** Link to related issues (e.g., "Closes #123")

4. **Default PR Description Template (if no template exists):**
   - Overview section with brief description
   - Changes section with bulleted list
   - Motivation section explaining why
   - Testing checklist (unit tests, integration tests, manual testing, all tests passing)
   - Screenshots/Examples section if applicable
   - Breaking Changes section if any
   - Related Issues section with issue links

5. **Create the PR:**
   - Use 'gh pr create' command with appropriate flags
   - Set the title and body based on the generated description
   - Set appropriate labels if needed
   - Request reviewers if applicable
   - Link to related issues

6. **Verify:**
   - Confirm the PR was created successfully
   - Provide the PR URL
   - Summarize the PR details

**Important:**
- Ensure you're on the correct branch before creating the PR
- Make sure all commits are pushed to the remote
- Verify the base branch is correct (usually 'main' or 'develop')
- Check that CI/CD checks are configured to run