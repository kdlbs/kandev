Please merge the base branch into the current branch and resolve any conflicts that arise.

**Merge Process:**

1. **Pre-merge checks:**
   - Identify the current branch name
   - Identify the base branch (usually 'main', 'master', or 'develop')
   - Check the current git status
   - Ensure working directory is clean (commit or stash changes if needed)
   - Fetch latest changes from remote

2. **Perform the merge:**
   - Execute: git fetch origin [base-branch]
   - Execute: git merge origin/[base-branch]
   - Check if there are any merge conflicts

3. **If conflicts exist:**
   - List all conflicting files
   - For each conflicting file:
     a. Show the conflict markers and surrounding context
     b. Analyze both versions (current branch vs base branch)
     c. Understand the intent of both changes
     d. Resolve the conflict by:
        - Keeping changes from current branch if they're the intended updates
        - Keeping changes from base branch if they're necessary updates
        - Combining both changes if they're complementary
        - Rewriting the section if neither version is correct
     e. Remove conflict markers (<<<<<<<, =======, >>>>>>>)
     f. Ensure the resolved code is syntactically correct
     g. Maintain code style and formatting consistency

4. **Post-resolution:**
   - Stage all resolved files: git add [resolved-files]
   - Verify no conflicts remain: git status
   - Run tests to ensure nothing broke:
     - Run linters if available
     - Run unit tests if available
     - Run build/compile if applicable
   - Complete the merge: git commit (or git merge --continue)

5. **Verification:**
   - Confirm merge was successful
   - Show the merge commit
   - List all files that were modified during conflict resolution
   - Summarize what conflicts were resolved and how

**Conflict Resolution Strategy:**
- **Understand context:** Always read the surrounding code to understand what each change is trying to accomplish
- **Preserve intent:** Keep changes that align with the feature/fix being developed
- **Maintain compatibility:** Ensure base branch updates (bug fixes, security patches) are preserved
- **Test thoroughly:** After resolution, verify the code works correctly
- **Document if needed:** Add comments if the resolution required complex logic

**Important Notes:**
- If conflicts are too complex or risky, ask for human review
- Never blindly accept one side without understanding both changes
- Ensure code quality and functionality are maintained
- If tests fail after merge, fix the issues before completing the merge