package project

import (
	"net/http"

	"superphenix-api/internal/authorization/permify"
	"superphenix-api/pkg/api/publicHttp/authentication"
	"superphenix-api/pkg/api/publicHttp/authentication/jwt"
	"superphenix-api/pkg/config"
	"superphenix-api/pkg/router"
	apiToken "superphenix-api/pkg/services/auth/apitoken"

	pwPermission "permify-wrapper/pkg/base/v1/permission"
)

const ModuleName = "project"

// API is the overridable seam for the project endpoints; the methods are the HTTP handlers.
type API interface {
	CreateOrUpdateProject(http.ResponseWriter, *http.Request)
	DeleteProject(http.ResponseWriter, *http.Request)
}

// Service is the default implementation of API.
type Service struct {
	cfg *config.Config
}

var _ API = (*Service)(nil)

// New constructs the default service. It has no side effects.
func New(cfg *config.Config) *Service { return &Service{cfg: cfg} }

// Module builds the route module for any API implementation. Routes dispatch
// through the interface and are mounted under "/v1" with their auth/permission chain.
func Module(s API) router.Module {
	var (
		jwtOrToken  = authentication.Authenticate(jwt.JwtBearerAuth, apiToken.ApiTokenAuth)
		orgaRead    = permify.CheckPermission(pwPermission.OrganizationRead)
		projectMgmt = permify.CheckPermission(pwPermission.OrganizationProjectManagement)
	)

	return router.Module{
		Name:  ModuleName,
		Mount: "/v1",
		Routes: []router.Route{
			router.Post("/organization/{orgaId}/project", s.CreateOrUpdateProject, jwtOrToken, orgaRead, projectMgmt),
			router.Delete("/organization/{orgaId}/project", s.DeleteProject, jwtOrToken, orgaRead, projectMgmt),
		},
	}
}

// ProvideService constructs the default service and registers its routes on reg.
func ProvideService(cfg *config.Config, reg *router.Registry) {
	h := New(cfg)
	reg.Register(Module(h))
}
