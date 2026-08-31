package az

import (
	"errors"
	"slices"

	"github.com/super-phenix/superphenix/internal/superphenix-api/pkg/config"
)

var ErrAZNotFound = errors.New("az not found")

func GetByCode(code string, orgaId string) (config.AZConfig, error) {
	az, ok := config.Global.AZs[code]
	if !ok {
		return config.AZConfig{}, ErrAZNotFound
	}
	if len(az.Whitelist) > 0 && !slices.Contains(az.Whitelist, orgaId) {
		return config.AZConfig{}, ErrAZNotFound
	}
	return az, nil
}

func FindAll(orgaId string) []config.AZConfig {
	var filtered []config.AZConfig
	for _, az := range config.Global.AZs {
		if len(az.Whitelist) == 0 {
			filtered = append(filtered, az)
		} else if slices.Contains(az.Whitelist, orgaId) {
			filtered = append(filtered, az)
		}
	}
	return filtered
}
