package worktree

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// ScriptExecutionRequest contains parameters for executing a setup or cleanup script.
type ScriptExecutionRequest struct {
	SessionID    string
	TaskID       string
	RepositoryID string
	Script       string
	WorkingDir   string
	ScriptType   string // "setup" or "cleanup"
}

// DefaultScriptMessageHandler manages script execution and streams output to session messages.
type DefaultScriptMessageHandler struct {
	logger      *logger.Logger
	taskService TaskService
	timeout     time.Duration
}

// TaskService interface for creating and updating messages.
type TaskService interface {
	CreateMessage(ctx context.Context, req *CreateMessageRequest) (*models.Message, error)
	UpdateMessage(ctx context.Context, message *models.Message) error
}

// CreateMessageRequest contains parameters for creating a message.
type CreateMessageRequest struct {
	TaskSessionID string
	TaskID        string
	TurnID        string
	Content       string
	AuthorType    string
	AuthorID      string
	RequestsInput bool
	Type          string
	Metadata      map[string]interface{}
}

// NewDefaultScriptMessageHandler creates a new DefaultScriptMessageHandler.
func NewDefaultScriptMessageHandler(
	log *logger.Logger,
	taskSvc TaskService,
	timeout time.Duration,
) *DefaultScriptMessageHandler {
	return &DefaultScriptMessageHandler{
		logger:      log.WithFields(zap.String("component", "script-message-handler")),
		taskService: taskSvc,
		timeout:     timeout,
	}
}

// ExecuteSetupScript executes a setup script and streams output to a session message.
// Returns an error if the script fails (non-zero exit code or timeout).
func (h *DefaultScriptMessageHandler) ExecuteSetupScript(ctx context.Context, req ScriptExecutionRequest) error {
	return h.executeScript(ctx, req, true)
}

// ExecuteCleanupScript executes a cleanup script and streams output to a session message.
// Returns nil even if the script fails (best-effort cleanup).
func (h *DefaultScriptMessageHandler) ExecuteCleanupScript(ctx context.Context, req ScriptExecutionRequest) error {
	err := h.executeScript(ctx, req, false)
	if err != nil {
		h.logger.Warn("cleanup script failed, continuing with deletion",
			zap.String("session_id", req.SessionID),
			zap.Error(err))
		return nil
	}
	return nil
}

// executeScript is the core implementation for script execution.
// Note: The parent context is intentionally not used - we create a detached context
// to prevent HTTP request timeouts from cancelling long-running scripts.
func (h *DefaultScriptMessageHandler) executeScript(_ context.Context, req ScriptExecutionRequest, failOnError bool) error {
	if h.taskService == nil {
		h.logger.Debug("script handler not fully configured, skipping",
			zap.String("script_type", req.ScriptType))
		return nil
	}

	// Create a detached context for script execution with its own timeout.
	// This prevents the HTTP request context from cancelling the script.
	scriptCtx, cancel := context.WithTimeout(context.Background(), h.timeout)
	defer cancel()

	// Create initial message (best-effort - session may not exist during cleanup)
	msg, err := h.createScriptMessage(scriptCtx, req)
	if err != nil {
		// For cleanup scripts, if session doesn't exist, run the script anyway without a message
		if req.ScriptType == "cleanup" {
			h.logger.Warn("failed to create cleanup script message, running script without message tracking",
				zap.String("session_id", req.SessionID),
				zap.Error(err))
			// Run script directly without message tracking
			return h.runScriptWithoutMessage(scriptCtx, req, failOnError)
		}
		// For setup scripts, this is a hard error
		return fmt.Errorf("failed to create script message: %w", err)
	}

	h.logger.Info("created script execution message",
		zap.String("message_id", msg.ID),
		zap.String("session_id", req.SessionID),
		zap.String("script_type", req.ScriptType))

	// Update message status to running
	msg.Metadata["status"] = "running"
	if err := h.taskService.UpdateMessage(scriptCtx, msg); err != nil {
		h.logger.Warn("failed to update message with running status",
			zap.String("message_id", msg.ID),
			zap.Error(err))
	}

	// Execute the script and capture output
	exitCode, scriptErr := h.runScriptWithOutput(scriptCtx, req, msg)

	// Update final status
	if scriptErr != nil {
		msg.Metadata["status"] = "failed"
		msg.Metadata["error"] = scriptErr.Error()
		if msg.Content == "" {
			msg.Content = fmt.Sprintf("Script execution failed: %s", scriptErr.Error())
		} else {
			msg.Content += fmt.Sprintf("\n\nScript execution failed: %s", scriptErr.Error())
		}
	} else if exitCode == 0 {
		msg.Metadata["status"] = "exited"
		msg.Metadata["exit_code"] = exitCode
		if msg.Content == "" {
			msg.Content = "Script completed successfully"
		}
	} else {
		msg.Metadata["status"] = "failed"
		msg.Metadata["exit_code"] = exitCode
		if msg.Content == "" {
			msg.Content = fmt.Sprintf("Script failed with exit code: %d", exitCode)
		} else {
			msg.Content += fmt.Sprintf("\n\nScript failed with exit code: %d", exitCode)
		}
	}

	msg.Metadata["completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	if updateErr := h.taskService.UpdateMessage(scriptCtx, msg); updateErr != nil {
		h.logger.Warn("failed to update message with final status",
			zap.String("message_id", msg.ID),
			zap.Error(updateErr))
	}

	h.logger.Info("script execution completed",
		zap.String("message_id", msg.ID),
		zap.Int("exit_code", exitCode),
		zap.Bool("success", exitCode == 0))

	// Return error if setup script failed
	if failOnError && (scriptErr != nil || exitCode != 0) {
		if scriptErr != nil {
			return scriptErr
		}
		return fmt.Errorf("script exited with code %d", exitCode)
	}

	return nil
}

// createScriptMessage creates the initial script execution message.
func (h *DefaultScriptMessageHandler) createScriptMessage(ctx context.Context, req ScriptExecutionRequest) (*models.Message, error) {
	metadata := map[string]interface{}{
		"script_type": req.ScriptType,
		"command":     req.Script,
		"status":      "starting",
		"started_at":  time.Now().UTC().Format(time.RFC3339Nano),
	}

	createReq := &CreateMessageRequest{
		TaskSessionID: req.SessionID,
		TaskID:        req.TaskID,
		Content:       "", // Will be populated with output
		AuthorType:    "agent",
		Type:          "script_execution",
		Metadata:      metadata,
	}

	return h.taskService.CreateMessage(ctx, createReq)
}

// runScriptWithOutput runs the script and captures output, streaming it to the message.
// The passed context should already have an appropriate timeout set.
func (h *DefaultScriptMessageHandler) runScriptWithOutput(ctx context.Context, req ScriptExecutionRequest, msg *models.Message) (int, error) {
	// Run script with sh -c to support complex commands
	cmd := exec.CommandContext(ctx, "sh", "-c", req.Script)
	cmd.Dir = req.WorkingDir

	// Create pipes for stdout and stderr
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return -1, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return -1, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return -1, fmt.Errorf("failed to start script: %w", err)
	}

	h.logger.Info("script process started",
		zap.String("message_id", msg.ID),
		zap.String("command", req.Script))

	// Stream output
	var outputBuf bytes.Buffer
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Stream stdout
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				mu.Lock()
				outputBuf.Write(buf[:n])
				msg.Content = outputBuf.String()
				mu.Unlock()

				// Update message with incremental output (best-effort)
				if updateErr := h.taskService.UpdateMessage(context.Background(), msg); updateErr != nil {
					h.logger.Debug("failed to update message with output",
						zap.String("message_id", msg.ID),
						zap.Error(updateErr))
				}
			}
			if err != nil {
				if err != io.EOF {
					h.logger.Debug("error reading stdout",
						zap.Error(err))
				}
				break
			}
		}
	}()

	// Stream stderr
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 1024)
		for {
			n, err := stderrPipe.Read(buf)
			if n > 0 {
				mu.Lock()
				outputBuf.Write(buf[:n])
				msg.Content = outputBuf.String()
				mu.Unlock()

				// Update message with incremental output (best-effort)
				if updateErr := h.taskService.UpdateMessage(context.Background(), msg); updateErr != nil {
					h.logger.Debug("failed to update message with output",
						zap.String("message_id", msg.ID),
						zap.Error(updateErr))
				}
			}
			if err != nil {
				if err != io.EOF {
					h.logger.Debug("error reading stderr",
						zap.Error(err))
				}
				break
			}
		}
	}()

	// Wait for output streaming to finish
	wg.Wait()

	// Wait for command to complete
	err = cmd.Wait()

	// Get exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			// Other error (e.g., timeout, command not found)
			return -1, err
		}
	}

	return exitCode, nil
}

// runScriptWithoutMessage runs a script without message tracking (used when session is deleted).
// The passed context should already have an appropriate timeout set.
func (h *DefaultScriptMessageHandler) runScriptWithoutMessage(ctx context.Context, req ScriptExecutionRequest, failOnError bool) error {
	h.logger.Info("executing script without message tracking",
		zap.String("script_type", req.ScriptType),
		zap.String("command", req.Script))

	// Run script with sh -c
	cmd := exec.CommandContext(ctx, "sh", "-c", req.Script)
	cmd.Dir = req.WorkingDir

	// Capture output for logging
	output, err := cmd.CombinedOutput()

	// Get exit code
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			h.logger.Error("script execution failed",
				zap.String("script_type", req.ScriptType),
				zap.Error(err),
				zap.String("output", string(output)))
			if failOnError {
				return err
			}
			return nil
		}
	}

	// Log the result
	if exitCode == 0 {
		h.logger.Info("script completed successfully",
			zap.String("script_type", req.ScriptType),
			zap.Int("exit_code", exitCode),
			zap.String("output", string(output)))
	} else {
		h.logger.Warn("script failed",
			zap.String("script_type", req.ScriptType),
			zap.Int("exit_code", exitCode),
			zap.String("output", string(output)))
		if failOnError {
			return fmt.Errorf("script exited with code %d", exitCode)
		}
	}

	return nil
}
