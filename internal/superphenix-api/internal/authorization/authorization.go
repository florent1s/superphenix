package authorization

import (
	"net/http"
	"slices"
	"superphenix-api/internal/consts"
	"superphenix-api/pkg/config"

	"permify-wrapper/pkg/base/v1/entity"
	pwV1 "permify-wrapper/pkg/base/v1/permission"
	pw "permify-wrapper/pkg/permission"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func CheckPermissionForRequest(r *http.Request, permission string) bool {
	orgaId := chi.URLParam(r, "orgaId")
	projectId := chi.URLParam(r, "projectId")
	userId := r.Context().Value(consts.ContextUserId)
	if userId == nil {
		log.Error().Msg("No user id found in context")
		return false
	}

	// Check for PM bypass
	if IsSuperAdminUser(userId.(string)) {
		log.Info().Ctx(r.Context()).
			Str("method", "CheckPermission").
			Str("url", r.URL.String()).
			Str("userId", userId.(string)).
			Msgf("Bypass permission check for permission %s", permission)
		return true
	}

	var entityId string
	switch pwV1.PermissionsEntityMap[permission] {
	case entity.Organization:
		entityId = orgaId
		break
	case entity.Project:
		entityId = projectId
		break
	}

	return pw.CanAccess(r.Context(), orgaId, permission, entityId, userId.(string))
}

func IsSuperAdminUser(userId string) bool {
	// Check for Super Admin bypass
	// If a whitelist is defined and not empty
	if config.Global.SuperAdmins != nil && len(config.Global.SuperAdmins) > 0 {
		// If the user is super admin
		if userId != "" && slices.Contains(config.Global.SuperAdmins, userId) {
			return true
		}
	}
	return false
}
