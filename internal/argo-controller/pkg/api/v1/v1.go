package publicHttp

import (
	"argo-controller/internal/utils"
	"argo-controller/pkg/api/authentication"
	"argo-controller/pkg/api/gc"
	"argo-controller/pkg/api/v1/argo"

	"github.com/go-chi/chi/v5"
)

func V1Router(r chi.Router) {
	r.Route("/{orgId}/{projectId}", func(r chi.Router) {
		r.Use(utils.AddUserToContext)
		r.Use(authentication.BearerAuth())
		r.Use(utils.CheckOrganizationWhitelist())

		// Mark for deletion endpoint (/mark)
		r.Get("/mark", gc.MarkForDeletion)

		argo.InitRouter(r)
	})
}
