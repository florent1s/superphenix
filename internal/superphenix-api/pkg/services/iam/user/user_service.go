package user

import (
	"net/http"

	"superphenix-api/pkg/api/publicHttp/authentication"
	"superphenix-api/pkg/api/publicHttp/authentication/jwt"
	"superphenix-api/pkg/config"
	"superphenix-api/pkg/router"
)

const moduleName = "user"

// API is the overridable seam for the user endpoints; the methods are the HTTP handlers.
type API interface {
	RegenerateInviteCode(http.ResponseWriter, *http.Request)
}

// Service is the default implementation of API.
type Service struct {
	cfg *config.Config
}

var _ API = (*Service)(nil)

// New constructs the default service. It has no side effects.
func New(cfg *config.Config) *Service { return &Service{cfg: cfg} }

// Module builds the route module for any API implementation. Routes
// dispatch through the interface and are mounted under "/v1" with their auth chain.
func Module(s API) router.Module {
	jwtAuth := authentication.Authenticate(jwt.JwtBearerAuth)

	return router.Module{
		Name:  moduleName,
		Mount: "/v1",
		Routes: []router.Route{
			router.Post("/invite-code", s.RegenerateInviteCode, jwtAuth),
		},
	}
}

// ProvideService constructs the default service and registers its routes on reg.
func ProvideService(cfg *config.Config, reg *router.Registry) {
	h := New(cfg)
	reg.Register(Module(h))
}
