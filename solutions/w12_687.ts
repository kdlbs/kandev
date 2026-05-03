// backend/handlers/github.go
package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"
)

type GitHubHandler struct {
	client      *GitHubClient
	cache       sync.Map
	sf          singleflight.Group
	cacheTTL    time.Duration
}

type PR struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Repo        string    `json:"repo"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	URL         string    `json:"url"`
}

type Issue struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Repo        string    `json:"repo"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	URL         string    `json:"url"`
}

type SearchQuery struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Query   string `json:"query"`
	Type    string `json:"type"` // "prs" or "issues"
}

type GitHubPageData struct {
	PRs          []PR          `json:"prs"`
	Issues       []Issue       `json:"issues"`
	SavedQueries []SearchQuery `json:"saved_queries"`
	TotalPRs     int           `json:"total_prs"`
	TotalIssues  int           `json:"total_issues"`
	Page         int           `json:"page"`
	PerPage      int           `json:"per_page"`
}

func NewGitHubHandler(client *GitHubClient) *GitHubHandler {
	return &GitHubHandler{
		client:   client,
		cacheTTL: 30 * time.Second,
	}
}

func (h *GitHubHandler) GetGitHubPage(c *gin.Context) {
	userID := c.GetString("user_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	repoFilter := c.Query("repo")
	query := c.Query("q")
	filterType := c.DefaultQuery("type", "all") // "prs", "issues", "all"

	cacheKey := h.buildCacheKey(userID, page, perPage, repoFilter, query, filterType)

	if data, ok := h.getFromCache(cacheKey); ok {
		c.JSON(http.StatusOK, data)
		return
	}

	data, err, _ := h.sf.Do(cacheKey, func() (interface{}, error) {
		return h.fetchGitHubData(userID, page, perPage, repoFilter, query, filterType)
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	h.setCache(cacheKey, data)
	c.JSON(http.StatusOK, data)
}

func (h *GitHubHandler) GetBatchPRStatus(c *gin.Context) {
	var prIDs []int
	if err := c.ShouldBindJSON(&prIDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	statuses, err := h.client.GetBatchPRStatus(c.Request.Context(), prIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, statuses)
}

func (h *GitHubHandler) SaveQuery(c *gin.Context) {
	var query SearchQuery
	if err := c.ShouldBindJSON(&query); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	userID := c.GetString("user_id")
	query.ID = h.client.GenerateQueryID()
	if err := h.client.SaveQuery(userID, query); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, query)
}

func (h *GitHubHandler) DeleteQuery(c *gin.Context) {
	queryID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid query ID"})
		return
	}

	userID := c.GetString("user_id")
	if err := h.client.DeleteQuery(userID, queryID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *GitHubHandler) LaunchTask(c *gin.Context) {
	var taskRequest struct {
		PRID    int    `json:"pr_id"`
		IssueID int    `json:"issue_id"`
		Repo    string `json:"repo"`
	}

	if err := c.ShouldBindJSON(&taskRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	userID := c.GetString("user_id")
	taskID, err := h.client.CreateTaskFromPRorIssue(userID, taskRequest.PRID, taskRequest.IssueID, taskRequest.Repo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"task_id": taskID})
}

func (h *GitHubHandler) fetchGitHubData(userID string, page, perPage int, repoFilter, query, filterType string) (*GitHubPageData, error) {
	var prs []PR
	var issues []Issue
	var totalPRs, totalIssues int
	var err error

	if filterType == "all" || filterType == "prs" {
		prs, totalPRs, err = h.client.GetUserPRs(userID, page, perPage, repoFilter, query)
		if err != nil {
			return nil, err
		}
	}

	if filterType == "all" || filterType == "issues" {
		issues, totalIssues, err = h.client.GetUserIssues(userID, page, perPage, repoFilter, query)
		if err != nil {
			return nil, err
		}
	}

	savedQueries, err := h.client.GetSavedQueries(userID)
	if err != nil {
		return nil, err
	}

	return &GitHubPageData{
		PRs:          prs,
		Issues:       issues,
		SavedQueries: savedQueries,
		TotalPRs:     totalPRs,
		TotalIssues:  totalIssues,
		Page:         page,
		PerPage:      perPage,
	}, nil
}

func (h *GitHubHandler) buildCacheKey(userID string, page, perPage int, repoFilter, query, filterType string) string {
	return userID + ":" + strconv.Itoa(page) + ":" + strconv.Itoa(perPage) + ":" + repoFilter + ":" + query + ":" + filterType
}

func (h *GitHubHandler) getFromCache(key string) (*GitHubPageData, bool) {
	if val, ok := h.cache.Load(key); ok {
		entry := val.(*cacheEntry)
		if time.Since(entry.timestamp) < h.cacheTTL {
			return entry.data, true
		}
		h.cache.Delete(key)
	}
	return nil, false
}

func (h *GitHubHandler) setCache(key string, data *GitHubPageData) {
	h.cache.Store(key, &cacheEntry{
		data:      data,
		timestamp: time.Now(),
	})
}

type cacheEntry struct {
	data      *GitHubPageData
	timestamp time.Time
}
