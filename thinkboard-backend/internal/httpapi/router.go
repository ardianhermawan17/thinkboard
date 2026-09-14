// Package httpapi is the Gin transport: router, middleware, handlers, DTOs (03-backend-folder-architecture.md §6).
package httpapi

import (
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"thinkboard-backend/internal/auth"
	"thinkboard-backend/internal/httpapi/handler"
	"thinkboard-backend/internal/httpapi/middleware"
	"thinkboard-backend/internal/platform/config"
	"thinkboard-backend/internal/shared/clock"
)

// NewRouter builds the route table. This is the only file that knows URLs (§6).
func NewRouter(cfg config.Config, db *pgxpool.Pool) *gin.Engine {
	r := gin.New() // not gin.Default() — middleware is explicit (§6)
	r.Use(middleware.Recover(), middleware.RequestID())

	health := &handler.HealthHandler{Clock: clock.Real}
	r.GET("/healthz", health.Health)

	// Swagger UI reads the hand-maintained spec directly — no swag codegen (api/openapi.yaml is
	// the source of truth per 03-backend-folder-architecture.md §6). The spec is served outside
	// /swagger/ because gin's router rejects a static file sibling under a catch-all prefix.
	r.StaticFile("/openapi.yaml", "api/openapi.yaml")
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.URL("/openapi.yaml")))

	verifier := auth.NewVerifier(cfg.SupabaseURL)
	authorizer := auth.NewCachedAuthorizer(auth.NewAuthorizer(db), cfg.MembershipCacheTTL, clock.Real)
	authMW := middleware.Auth(verifier, authorizer)

	v1 := r.Group("/v1")
	v1.Use(authMW)
	{
		me := &handler.MeHandler{}
		v1.GET("/me", me.Me)
	}

	return r
}
