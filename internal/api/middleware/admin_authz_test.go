package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"

	"github.com/moistello/backend/internal/api/middleware"
)

// TestAdminAuthzMatrix_AdminToken_ReachesAdminEndpoint verifies that a JWT with
// role=admin passes both AuthMiddleware and AdminMiddleware, reaching the handler.
func TestAdminAuthzMatrix_AdminToken_ReachesAdminEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keys := newRSATestKeys(t)

	claims := &middleware.Claims{
		UserID: "admin-user-1",
		Wallet: "GADMIN...",
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tokenString := signRS256(t, keys.privateKey, claims)

	r := gin.New()
	r.Use(middleware.AuthMiddleware(keys.publicKeyPEM))
	admin := r.Group("/admin")
	admin.Use(middleware.AdminMiddleware())
	admin.GET("/users", func(c *gin.Context) {
		c.JSON(200, gin.H{"role": middleware.GetRole(c), "userID": middleware.GetUserID(c)})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "admin-user-1")
	assert.Contains(t, w.Body.String(), `"role":"admin"`)
}

// TestAdminAuthzMatrix_NonAdminToken_Rejected verifies that a JWT with role=user
// passes AuthMiddleware but is blocked by AdminMiddleware with 403.
func TestAdminAuthzMatrix_NonAdminToken_Rejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keys := newRSATestKeys(t)

	claims := &middleware.Claims{
		UserID: "regular-user-1",
		Wallet: "GUSER...",
		Role:   "user",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tokenString := signRS256(t, keys.privateKey, claims)

	r := gin.New()
	r.Use(middleware.AuthMiddleware(keys.publicKeyPEM))
	admin := r.Group("/admin")
	admin.Use(middleware.AdminMiddleware())
	admin.GET("/users", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	r.ServeHTTP(w, req)

	assert.Equal(t, 403, w.Code)
	assert.Contains(t, w.Body.String(), "admin access required")
}

// TestAdminAuthzMatrix_NoToken_Rejected verifies that requests without a token
// are rejected at the AuthMiddleware level with 401.
func TestAdminAuthzMatrix_NoToken_Rejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keys := newRSATestKeys(t)

	r := gin.New()
	r.Use(middleware.AuthMiddleware(keys.publicKeyPEM))
	admin := r.Group("/admin")
	admin.Use(middleware.AdminMiddleware())
	admin.GET("/users", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/users", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

// TestAdminAuthzMatrix_ExpiredAdminToken_Rejected verifies that an expired admin
// token is rejected at the AuthMiddleware level.
func TestAdminAuthzMatrix_ExpiredAdminToken_Rejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keys := newRSATestKeys(t)

	claims := &middleware.Claims{
		UserID: "admin-user-1",
		Wallet: "GADMIN...",
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	tokenString := signRS256(t, keys.privateKey, claims)

	r := gin.New()
	r.Use(middleware.AuthMiddleware(keys.publicKeyPEM))
	admin := r.Group("/admin")
	admin.Use(middleware.AdminMiddleware())
	admin.GET("/users", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

// TestAdminAuthzMatrix_EmptyRole_Rejected verifies that a token with no role
// is rejected by AdminMiddleware.
func TestAdminAuthzMatrix_EmptyRole_Rejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keys := newRSATestKeys(t)

	claims := &middleware.Claims{
		UserID: "user-no-role",
		Wallet: "GWALLET...",
		Role:   "", // Empty role
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tokenString := signRS256(t, keys.privateKey, claims)

	r := gin.New()
	r.Use(middleware.AuthMiddleware(keys.publicKeyPEM))
	admin := r.Group("/admin")
	admin.Use(middleware.AdminMiddleware())
	admin.GET("/users", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/admin/users", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	r.ServeHTTP(w, req)

	assert.Equal(t, 403, w.Code)
}

// TestAdminAuthzMatrix_MultipleAdminEndpoints verifies the matrix across
// several admin endpoints to ensure consistent enforcement.
func TestAdminAuthzMatrix_MultipleAdminEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	keys := newRSATestKeys(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/admin/users"},
		{"GET", "/admin/circles"},
		{"GET", "/admin/audit-log"},
		{"GET", "/admin/metrics"},
	}

	// Admin token — should pass all
	adminClaims := &middleware.Claims{
		UserID: "admin-user",
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	adminToken := signRS256(t, keys.privateKey, adminClaims)

	// Regular user token — should fail all
	userClaims := &middleware.Claims{
		UserID: "regular-user",
		Role:   "user",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	userToken := signRS256(t, keys.privateKey, userClaims)

	r := gin.New()
	r.Use(middleware.AuthMiddleware(keys.publicKeyPEM))
	admin := r.Group("/admin")
	admin.Use(middleware.AdminMiddleware())
	for _, ep := range endpoints {
		path := ep.path
		switch ep.method {
		case "GET":
			admin.GET(path[len("/admin"):], func(c *gin.Context) {
				c.JSON(200, gin.H{"ok": true})
			})
		}
	}

	for _, ep := range endpoints {
		t.Run("admin_"+ep.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			r.ServeHTTP(w, req)
			assert.Equal(t, 200, w.Code, "admin should reach %s", ep.path)
		})

		t.Run("user_"+ep.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			req.Header.Set("Authorization", "Bearer "+userToken)
			r.ServeHTTP(w, req)
			assert.Equal(t, 403, w.Code, "regular user should be rejected from %s", ep.path)
		})

		t.Run("noauth_"+ep.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest(ep.method, ep.path, nil)
			r.ServeHTTP(w, req)
			assert.Equal(t, 401, w.Code, "unauthenticated should be rejected from %s", ep.path)
		})
	}
}

// TestAdminAuthzMatrix_APIKey_ValidKey verifies admin API key access works.
func TestAdminAuthzMatrix_APIKey_ValidKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware.AdminAPIKeyMiddleware("secret-admin-key"))
	r.GET("/metrics", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	req.Header.Set("X-Admin-API-Key", "secret-admin-key")
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code)
}

// TestAdminAuthzMatrix_APIKey_InvalidKey verifies wrong API key is rejected.
func TestAdminAuthzMatrix_APIKey_InvalidKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware.AdminAPIKeyMiddleware("secret-admin-key"))
	r.GET("/metrics", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	req.Header.Set("X-Admin-API-Key", "wrong-key")
	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}

// TestAdminAuthzMatrix_APIKey_MissingKey verifies missing API key is rejected.
func TestAdminAuthzMatrix_APIKey_MissingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := gin.New()
	r.Use(middleware.AdminAPIKeyMiddleware("secret-admin-key"))
	r.GET("/metrics", func(c *gin.Context) {
		c.JSON(200, gin.H{"ok": true})
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/metrics", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, 401, w.Code)
}
