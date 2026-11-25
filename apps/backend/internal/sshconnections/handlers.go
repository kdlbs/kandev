package sshconnections

import (
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/authz"
	"io"
	"net/http"
)

// RegisterRoutes exposes only administrative operations. Disabled installations
// register no manager routes.
func RegisterRoutes(router *gin.Engine, s *Service) {
	if s == nil || !s.enabled {
		return
	}
	group := router.Group("/api/v1/ssh/managed", authz.RequireOrgScope(authz.ScopeOrgConfigManage))
	group.GET("/identities", func(c *gin.Context) { value, err := s.ListIdentities(c.Request.Context()); respond(c, value, err) })
	group.POST("/identities", func(c *gin.Context) {
		var req CreateIdentityRequest
		if !decode(c, &req) {
			return
		}
		value, err := s.CreateIdentity(c.Request.Context(), req)
		respond(c, value, err)
	})
	group.PATCH("/identities/:id", func(c *gin.Context) {
		var req struct {
			Label   string `json:"label"`
			Version int64  `json:"expected_version"`
		}
		if !decode(c, &req) {
			return
		}
		value, err := s.RenameIdentity(c.Request.Context(), c.Param("id"), req.Label, req.Version)
		respond(c, value, err)
	})
	group.DELETE("/identities/:id", func(c *gin.Context) {
		var req struct {
			Version int64 `json:"expected_version"`
		}
		if !decode(c, &req) {
			return
		}
		err := s.DeleteIdentity(c.Request.Context(), c.Param("id"), req.Version)
		respond(c, gin.H{"deleted": err == nil}, err)
	})
	group.POST("/connections", func(c *gin.Context) {
		var req SaveConnectionRequest
		if !decode(c, &req) {
			return
		}
		if req.ExpectedVersion != 0 {
			respond(c, nil, ErrInvalidRoute)
			return
		}
		value, err := s.SaveConnection(c.Request.Context(), req)
		respond(c, value, err)
	})
	group.PATCH("/connections/:id", func(c *gin.Context) {
		var req SaveConnectionRequest
		if !decode(c, &req) {
			return
		}
		if req.ExpectedVersion < 1 || req.Connection.ID != c.Param("id") {
			respond(c, nil, ErrInvalidRoute)
			return
		}
		value, err := s.SaveConnection(c.Request.Context(), req)
		respond(c, value, err)
	})
	registerEnrollmentRoutes(group, s)
	registerReadRoutes(group, s)
	registerTrustRoutes(group, s)
}
func registerTrustRoutes(group *gin.RouterGroup, s *Service) {
	group.POST("/connections/:id/preflight", func(c *gin.Context) {
		var req struct {
			Stamp string `json:"expected_stamp"`
		}
		if !decode(c, &req) {
			return
		}
		value, err := s.Preflight(c.Request.Context(), c.Param("id"), req.Stamp)
		respond(c, value, err)
	})
	group.POST("/connections/:id/observe", func(c *gin.Context) {
		var req struct {
			Stamp string `json:"expected_stamp"`
		}
		if !decode(c, &req) {
			return
		}
		value, err := s.Observe(c.Request.Context(), c.Param("id"), req.Stamp)
		respond(c, value, err)
	})
	group.POST("/connections/:id/trust", func(c *gin.Context) {
		var req struct {
			Stamp       string `json:"expected_stamp"`
			Receipt     string `json:"receipt"`
			Fingerprint string `json:"fingerprint"`
		}
		if !decode(c, &req) {
			return
		}
		value, err := s.Trust(c.Request.Context(), c.Param("id"), req.Stamp, req.Receipt, req.Fingerprint)
		respond(c, value, err)
	})
	group.POST("/connections/:id/test", func(c *gin.Context) {
		var req struct {
			Stamp string `json:"expected_stamp"`
		}
		if !decode(c, &req) {
			return
		}
		value, err := s.TestConnection(c.Request.Context(), c.Param("id"), req.Stamp)
		respond(c, value, err)
	})
}
func decode(c *gin.Context, value any) bool {
	reader := http.MaxBytesReader(c.Writer, c.Request.Body, 256*1024)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{errorResponseKey: "ssh_request_invalid"})
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{errorResponseKey: "ssh_request_invalid"})
		return false
	}
	return true
}
func respond(c *gin.Context, value any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, value)
		return
	}
	var credential *CredentialFailure
	var inUse *IdentityInUse
	if errors.As(err, &inUse) {
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{errorResponseKey: ErrInUse.Error(), "dependencies": inUse.Dependencies})
		return
	}
	if errors.As(err, &credential) {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: credential.Code})
		return
	}
	var failure *ProbeFailure
	if errors.As(err, &failure) {
		c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{errorResponseKey: failure.Code, "hop": failure.Hop})
		return
	}
	code, status := "ssh_operation_failed", http.StatusServiceUnavailable
	for _, entry := range []struct {
		err    error
		status int
	}{
		{ErrDisabled, 404}, {ErrNotFound, 404}, {ErrForbidden, 403}, {ErrConflict, 409}, {ErrInUse, 409}, {ErrInvalidRoute, 400}, {ErrInvalidIdentity, 400}, {ErrTrust, 422}, {ErrCredential, 422}, {ErrBastionNotReady, 422}, {ErrConnectionFailed, 503},
	} {
		if errors.Is(err, entry.err) {
			code = entry.err.Error()
			status = entry.status
			break
		}
	}
	c.AbortWithStatusJSON(status, gin.H{errorResponseKey: code})
}

func registerReadRoutes(group *gin.RouterGroup, s *Service) {
	group.GET("/connections", func(c *gin.Context) { value, err := s.ListConnections(c.Request.Context()); respond(c, value, err) })
	group.GET("/connections/:id", func(c *gin.Context) {
		value, err := s.GetConnection(c.Request.Context(), c.Param("id"))
		respond(c, value, err)
	})
	group.GET("/identities/:id", func(c *gin.Context) {
		value, err := s.GetIdentity(c.Request.Context(), c.Param("id"))
		respond(c, value, err)
	})
	group.DELETE("/connections/:id", func(c *gin.Context) {
		var req struct {
			Version int64 `json:"expected_version"`
		}
		if !decode(c, &req) {
			return
		}
		err := s.DeleteConnection(c.Request.Context(), c.Param("id"), req.Version)
		respond(c, gin.H{"deleted": err == nil}, err)
	})
}

func registerEnrollmentRoutes(group *gin.RouterGroup, s *Service) {
	group.POST("/connections/:id/enroll", func(c *gin.Context) {
		var req EnrollmentRequest
		if !decode(c, &req) {
			return
		}
		value, err := s.Enroll(c.Request.Context(), c.Param("id"), req)
		respond(c, value, err)
	})
	group.GET("/connections/:id/enrollments/:request_id", func(c *gin.Context) {
		value, err := s.GetEnrollment(c.Request.Context(), c.Param("id"), c.Param("request_id"))
		respond(c, value, err)
	})
}
