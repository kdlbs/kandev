package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/turnchanges"
)

type turnCheckpointErrorResponse struct {
	Error  string                 `json:"error"`
	Reason turnchanges.ReasonCode `json:"reason,omitempty"`
}

type turnCheckpointScopesResponse struct {
	Scopes []string `json:"scopes"`
}

func (s *Server) handleGitTurnCheckpointScopes(c *gin.Context) {
	scopes := s.procMgr.RepositoryScopes()
	if scopes == nil {
		scopes = []string{}
	}
	c.JSON(http.StatusOK, turnCheckpointScopesResponse{Scopes: scopes})
}

func (s *Server) handleGitTurnCheckpoint(c *gin.Context) {
	var request turnchanges.CheckpointRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, turnCheckpointErrorResponse{Error: "invalid request: " + err.Error()})
		return
	}
	if request.ChangeSetID == "" || request.CheckoutID == "" ||
		(request.Boundary != turnchanges.CheckpointStart && request.Boundary != turnchanges.CheckpointEnd) {
		c.JSON(http.StatusBadRequest, turnCheckpointErrorResponse{Error: "change_set_id, checkout_id, and a valid boundary are required"})
		return
	}
	operator, ok := s.registeredTurnChangeGitOperator(c, request.Repo)
	if !ok {
		return
	}
	result, err := operator.CaptureTurnCheckpoint(c.Request.Context(), request)
	if err != nil {
		writeTurnCheckpointError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) handleGitTurnCheckpointDelete(c *gin.Context) {
	var request turnchanges.CheckpointDeleteRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, turnCheckpointErrorResponse{Error: "invalid request: " + err.Error()})
		return
	}
	if request.ChangeSetID == "" || request.CheckoutID == "" || request.CommitOID == "" ||
		(request.Boundary != turnchanges.CheckpointStart && request.Boundary != turnchanges.CheckpointEnd) {
		c.JSON(http.StatusBadRequest, turnCheckpointErrorResponse{Error: "change-set, checkout, boundary, and expected commit identity are required"})
		return
	}
	operator, ok := s.registeredTurnChangeGitOperator(c, request.Repo)
	if !ok {
		return
	}
	if err := operator.DeleteTurnCheckpoint(c.Request.Context(), request); err != nil {
		writeTurnCheckpointError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (s *Server) handleGitTurnCheckpointCompare(c *gin.Context) {
	handleAcceptedTurnCheckpointQuery(s, c, &turnchanges.CompareRequest{},
		func(request *turnchanges.CompareRequest) acceptedTurnCheckpointQuery {
			return acceptedTurnCheckpointQuery{
				changeSetID: request.ChangeSetID, checkoutID: request.CheckoutID, repo: request.Repo,
				hashAlgorithm: request.HashAlgorithm, startCommitOID: request.StartCommitOID,
				startTreeOID: request.StartTreeOID, endCommitOID: request.EndCommitOID, endTreeOID: request.EndTreeOID,
			}
		},
		func(operator *process.GitOperator, request *turnchanges.CompareRequest) (any, error) {
			return operator.CompareTurnCheckpoints(c.Request.Context(), *request)
		})
}

func (s *Server) handleGitTurnCheckpointExport(c *gin.Context) {
	handleAcceptedTurnCheckpointQuery(s, c, &turnchanges.ExportRequest{},
		func(request *turnchanges.ExportRequest) acceptedTurnCheckpointQuery {
			return acceptedTurnCheckpointQuery{
				changeSetID: request.ChangeSetID, checkoutID: request.CheckoutID, repo: request.Repo,
				hashAlgorithm: request.HashAlgorithm, startCommitOID: request.StartCommitOID,
				startTreeOID: request.StartTreeOID, endCommitOID: request.EndCommitOID, endTreeOID: request.EndTreeOID,
			}
		},
		func(operator *process.GitOperator, request *turnchanges.ExportRequest) (any, error) {
			return operator.ExportTurnCheckpoint(c.Request.Context(), *request)
		})
}

type acceptedTurnCheckpointQuery struct {
	changeSetID, checkoutID, repo string
	hashAlgorithm                 string
	startCommitOID, startTreeOID  string
	endCommitOID, endTreeOID      string
}

func handleAcceptedTurnCheckpointQuery[T any](
	s *Server,
	c *gin.Context,
	request *T,
	identity func(*T) acceptedTurnCheckpointQuery,
	operation func(*process.GitOperator, *T) (any, error),
) {
	if err := c.ShouldBindJSON(request); err != nil {
		c.JSON(http.StatusBadRequest, turnCheckpointErrorResponse{Error: "invalid request: " + err.Error()})
		return
	}
	query := identity(request)
	if query.changeSetID == "" || query.checkoutID == "" || !hasAcceptedCheckpointPair(
		query.hashAlgorithm, query.startCommitOID, query.startTreeOID, query.endCommitOID, query.endTreeOID,
	) {
		c.JSON(http.StatusBadRequest, turnCheckpointErrorResponse{Error: "change-set, checkout, and accepted endpoint identities are required"})
		return
	}
	operator, ok := s.registeredTurnChangeGitOperator(c, query.repo)
	if !ok {
		return
	}
	result, err := operation(operator, request)
	if err != nil {
		writeTurnCheckpointError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func hasAcceptedCheckpointPair(hashAlgorithm, startCommit, startTree, endCommit, endTree string) bool {
	return (hashAlgorithm == "sha1" || hashAlgorithm == "sha256") &&
		startCommit != "" && startTree != "" && endCommit != "" && endTree != ""
}

func (s *Server) registeredTurnChangeGitOperator(c *gin.Context, repo string) (*process.GitOperator, bool) {
	for _, registered := range s.procMgr.RepositoryScopes() {
		if registered == repo {
			operator, err := s.procMgr.GitOperatorFor(repo)
			if err != nil {
				c.JSON(http.StatusBadRequest, turnCheckpointErrorResponse{Error: err.Error()})
				return nil, false
			}
			return operator, true
		}
	}
	c.JSON(http.StatusBadRequest, turnCheckpointErrorResponse{Error: "repository scope is not registered"})
	return nil, false
}

func writeTurnCheckpointError(c *gin.Context, err error) {
	var checkpointErr *process.TurnCheckpointError
	if !errors.As(err, &checkpointErr) {
		status := http.StatusUnprocessableEntity
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		c.JSON(status, turnCheckpointErrorResponse{Error: err.Error()})
		return
	}
	status := http.StatusUnprocessableEntity
	switch checkpointErr.Reason {
	case turnchanges.ReasonUnsafeGitState:
		status = http.StatusBadRequest
	case turnchanges.ReasonUnsupportedExecutor:
		status = http.StatusNotImplemented
	case turnchanges.ReasonCheckoutUnavailable:
		status = http.StatusServiceUnavailable
	}
	if errors.Is(err, context.DeadlineExceeded) {
		status = http.StatusGatewayTimeout
	}
	c.JSON(status, turnCheckpointErrorResponse{Error: err.Error(), Reason: checkpointErr.Reason})
}
