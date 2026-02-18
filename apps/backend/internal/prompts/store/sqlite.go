package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/prompts/models"
)

type sqliteRepository struct {
	db     *sqlx.DB // writer
	ro     *sqlx.DB // reader
	ownsDB bool
}

func newSQLiteRepositoryWithDB(writer, reader *sqlx.DB) (*sqliteRepository, error) {
	return newSQLiteRepository(writer, reader, false)
}

func newSQLiteRepository(writer, reader *sqlx.DB, ownsDB bool) (*sqliteRepository, error) {
	repo := &sqliteRepository{db: writer, ro: reader, ownsDB: ownsDB}
	if err := repo.initSchema(); err != nil {
		if ownsDB {
			if closeErr := writer.Close(); closeErr != nil {
				return nil, fmt.Errorf("failed to close database after schema error: %w", closeErr)
			}
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return repo, nil
}

func (r *sqliteRepository) initSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS custom_prompts (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			content TEXT NOT NULL,
			builtin INTEGER NOT NULL DEFAULT 0,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
	`
	if _, err := r.db.Exec(schema); err != nil {
		return err
	}

	// Seed built-in prompts
	if err := r.seedBuiltinPrompts(); err != nil {
		return fmt.Errorf("failed to seed built-in prompts: %w", err)
	}

	return nil
}

func (r *sqliteRepository) Close() error {
	if !r.ownsDB {
		return nil
	}
	return r.db.Close()
}

func (r *sqliteRepository) ListPrompts(ctx context.Context) ([]*models.Prompt, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT id, name, content, builtin, created_at, updated_at
		FROM custom_prompts
		ORDER BY builtin DESC, name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var prompts []*models.Prompt
	for rows.Next() {
		prompt := &models.Prompt{}
		var builtinInt int
		if err := rows.Scan(&prompt.ID, &prompt.Name, &prompt.Content, &builtinInt, &prompt.CreatedAt, &prompt.UpdatedAt); err != nil {
			return nil, err
		}
		prompt.Builtin = builtinInt == 1
		prompts = append(prompts, prompt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return prompts, nil
}

func (r *sqliteRepository) GetPromptByID(ctx context.Context, id string) (*models.Prompt, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, name, content, builtin, created_at, updated_at
		FROM custom_prompts
		WHERE id = ?
	`), id)
	prompt := &models.Prompt{}
	var builtinInt int
	if err := row.Scan(&prompt.ID, &prompt.Name, &prompt.Content, &builtinInt, &prompt.CreatedAt, &prompt.UpdatedAt); err != nil {
		return nil, err
	}
	prompt.Builtin = builtinInt == 1
	return prompt, nil
}

func (r *sqliteRepository) GetPromptByName(ctx context.Context, name string) (*models.Prompt, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, name, content, builtin, created_at, updated_at
		FROM custom_prompts
		WHERE name = ?
	`), name)
	prompt := &models.Prompt{}
	var builtinInt int
	if err := row.Scan(&prompt.ID, &prompt.Name, &prompt.Content, &builtinInt, &prompt.CreatedAt, &prompt.UpdatedAt); err != nil {
		return nil, err
	}
	prompt.Builtin = builtinInt == 1
	return prompt, nil
}

func (r *sqliteRepository) CreatePrompt(ctx context.Context, prompt *models.Prompt) error {
	if prompt.ID == "" {
		prompt.ID = uuid.New().String()
	}
	prompt.Name = strings.TrimSpace(prompt.Name)
	prompt.Content = strings.TrimSpace(prompt.Content)
	if prompt.CreatedAt.IsZero() {
		prompt.CreatedAt = time.Now().UTC()
	}
	prompt.UpdatedAt = time.Now().UTC()

	builtinInt := 0
	if prompt.Builtin {
		builtinInt = 1
	}

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO custom_prompts (id, name, content, builtin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), prompt.ID, prompt.Name, prompt.Content, builtinInt, prompt.CreatedAt, prompt.UpdatedAt)
	return err
}

func (r *sqliteRepository) UpdatePrompt(ctx context.Context, prompt *models.Prompt) error {
	if prompt == nil {
		return errors.New("prompt is nil")
	}
	prompt.Name = strings.TrimSpace(prompt.Name)
	prompt.Content = strings.TrimSpace(prompt.Content)
	prompt.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE custom_prompts
		SET name = ?, content = ?, updated_at = ?
		WHERE id = ?
	`), prompt.Name, prompt.Content, prompt.UpdatedAt, prompt.ID)
	return err
}

func (r *sqliteRepository) DeletePrompt(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM custom_prompts WHERE id = ?`), id)
	return err
}

// seedBuiltinPrompts creates the default built-in prompts if they don't exist
func (r *sqliteRepository) seedBuiltinPrompts() error {
	builtinPrompts := r.getBuiltinPrompts()

	for _, prompt := range builtinPrompts {
		// Check if prompt already exists
		var exists bool
		err := r.db.QueryRow(r.db.Rebind("SELECT 1 FROM custom_prompts WHERE id = ?"), prompt.ID).Scan(&exists)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if exists {
			continue
		}

		// Insert built-in prompt
		_, err = r.db.Exec(r.db.Rebind(`
			INSERT INTO custom_prompts (id, name, content, builtin, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`), prompt.ID, prompt.Name, prompt.Content, 1, prompt.CreatedAt, prompt.UpdatedAt)
		if err != nil {
			return fmt.Errorf("failed to insert built-in prompt %s: %w", prompt.ID, err)
		}
	}

	return nil
}

// getBuiltinPrompts returns the predefined built-in prompts
func (r *sqliteRepository) getBuiltinPrompts() []*models.Prompt {
	now := time.Now().UTC()
	return []*models.Prompt{
		r.builtinCodeReviewPrompt(now),
		r.builtinOpenPRPrompt(now),
		r.builtinMergeBasePrompt(now),
	}
}

func (r *sqliteRepository) builtinCodeReviewPrompt(now time.Time) *models.Prompt {
	return &models.Prompt{
		ID:        "builtin-code-review",
		Name:      "code-review",
		Builtin:   true,
		CreatedAt: now,
		UpdatedAt: now,
		Content: `Please review the changed files in the current git worktree.

STEP 1: Determine what to review
- First, check if there are any uncommitted changes (dirty working directory)
- If there are uncommitted/staged changes: review those files
- If the working directory is clean: review the commits that have diverged from the master/main branch

STEP 2: Review the changes, then output your findings in EXACTLY 4 sections: BUG, IMPROVEMENT, NITPICK, PERFORMANCE.

Rules:
- Each section is OPTIONAL - only include it if you have findings for that category
- If a section has no findings, omit it entirely
- Format each finding as: filename:line_number - Description
- Be specific and reference exact line numbers
- Keep descriptions concise but actionable
- Sort findings by severity within each section
- Focus on logic and design issues, NOT formatting or style that automated tools handle

Section definitions:

BUG: Critical issues that will cause runtime errors, crashes, incorrect behavior, data corruption, or logic errors
- Examples: null/nil dereference, race conditions, incorrect algorithms, type mismatches, resource leaks, deadlocks

IMPROVEMENT: Code quality, architecture, security, or maintainability concerns
- Examples: missing error handling, incorrect access modifiers (public/private/exported), SQL injection vulnerabilities, hardcoded credentials, tight coupling, missing validation, incorrect concurrency patterns

NITPICK: Significant readability or maintainability issues that impact code understanding
- Examples: misleading variable/function names, overly complex logic that should be refactored, missing critical comments for complex algorithms, inconsistent error handling patterns
- EXCLUDE: formatting, whitespace, import ordering, trivial naming preferences, style issues handled by linters/formatters

PERFORMANCE: Algorithmic or resource usage problems with measurable impact
- Examples: O(n²) where O(n) or O(1) is possible, unnecessary allocations in loops, missing indexes for database queries, blocking I/O in hot paths, regex compilation in loops, unbounded resource growth
- Concurrency-specific: unprotected shared state, missing synchronization, improper use of locks, goroutine leaks, missing context cancellation
- Prefer structured concurrency libraries (e.g., errgroup, conc) over raw primitives for better error handling and panic recovery

Example format:
## BUG
- src/handler.go:45 - Dereferencing pointer without nil check will panic when user is not found
- lib/parser.rs:123 - Loop condition uses <= instead of < causing out-of-bounds access

## IMPROVEMENT
- api/db.go:67 - Database query error ignored, will silently fail and return stale data
- services/auth.py:34 - Password comparison vulnerable to timing attacks, use constant-time comparison
- internal/user.go:15 - Type exported but only used internally, should be unexported

## NITPICK
- components/processor.ts:12 - Function name 'doStuff' doesn't describe what it actually does (transforms user input to API format)
- utils/cache.go:89 - Error wrapped multiple times making original cause hard to trace

## PERFORMANCE
- src/repository.go:156 - Linear search through slice on every request, use map for O(1) lookup
- handlers/api.py:45 - Compiling regex inside handler function, compile once at module level
- workers/processor.go:78 - Launching unbounded goroutines without limit, use worker pool or semaphore pattern
- db/queries.go:34 - N+1 query pattern, fetch all related records in single query with join

Now review the changes.`,
	}
}

func (r *sqliteRepository) builtinOpenPRPrompt(now time.Time) *models.Prompt {
	return &models.Prompt{
		ID:        "builtin-open-pr",
		Name:      "open-pr",
		Builtin:   true,
		CreatedAt: now,
		UpdatedAt: now,
		Content: `Please create and open a Pull Request for the current branch using the GitHub CLI (gh).

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
   - **Title:** Clear, concise summary of the changes (50-72 characters)
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
- Check that CI/CD checks are configured to run`,
	}
}

func (r *sqliteRepository) builtinMergeBasePrompt(now time.Time) *models.Prompt {
	return &models.Prompt{
		ID:        "builtin-merge-base",
		Name:      "merge-base",
		Builtin:   true,
		CreatedAt: now,
		UpdatedAt: now,
		Content: `Please merge the base branch into the current branch and resolve any conflicts that arise.

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
- If tests fail after merge, fix the issues before completing the merge`,
	}
}
