package worktree

import (
	"context"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/gitcheckout"
	"github.com/kandev/kandev/internal/task/models"
)

type checkoutContextKey struct{}
type checkoutContext struct {
	options *models.RepositoryCheckoutOptions
	env     []string
}

func withCheckoutOptions(ctx context.Context, req CreateRequest) (context.Context, error) {
	options, err := models.NormalizeRepositoryCheckoutOptions(req.CheckoutOptions)
	if err != nil || options == nil {
		return ctx, err
	}
	env := os.Environ()
	keys := make([]string, 0, len(req.ScriptEnv))
	for key := range req.ScriptEnv {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env = append(env, key+"="+req.ScriptEnv[key])
	}
	return context.WithValue(ctx, checkoutContextKey{}, checkoutContext{options: options, env: env}), nil
}

func scopedCheckout(ctx context.Context) checkoutContext {
	scope, _ := ctx.Value(checkoutContextKey{}).(checkoutContext)
	return scope
}

func hasSparseCheckout(ctx context.Context) bool {
	options := scopedCheckout(ctx).options
	return options != nil && len(options.SparseDirectories) > 0
}

func configureCheckoutCommand(ctx context.Context, cmd *exec.Cmd) {
	if scope := scopedCheckout(ctx); scope.options != nil {
		cmd.Env = append([]string(nil), scope.env...)
	}
}

func applySparseCheckout(ctx context.Context, path string) error {
	return gitcheckout.ApplySparse(ctx, path, scopedCheckout(ctx).options, func(ctx context.Context, args []string, input string) ([]byte, error) {
		cmd := newGitCommand(ctx, args...)
		cmd.Stdin = strings.NewReader(input)
		return runGitCmdCombinedOutput(ctx, cmd)
	})
}

func (m *Manager) finishWorktreeCheckout(ctx context.Context, repoPath, worktreePath string, usesGitCrypt bool) error {
	var err error
	if usesGitCrypt {
		err = m.unlockGitCryptAndCheckout(ctx, worktreePath)
	} else {
		err = applySparseCheckout(ctx, worktreePath)
	}
	if err == nil {
		err = gitcheckout.Save(worktreePath, scopedCheckout(ctx).options)
	}
	if err != nil {
		_ = m.removeWorktreeDir(ctx, worktreePath, repoPath)
		return err
	}
	if !usesGitCrypt {
		m.initSubmodules(ctx, worktreePath)
	}
	return nil
}
