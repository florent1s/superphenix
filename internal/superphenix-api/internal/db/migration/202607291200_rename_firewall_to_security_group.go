package migration

import (
	"github.com/super-phenix/superphenix/internal/superphenix-api/internal/db/model"

	"github.com/go-gormigrate/gormigrate/v2"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

var migration202607291200 = &gormigrate.Migration{
	ID: "202607291200_rename_firewall_to_security_group",
	Migrate: func(tx *gorm.DB) error {
		// product_types.id is referenced by products.product_type_id, so
		// insert the new row, repoint products, then drop the old row.
		if res := tx.Create(&model.ProductType{ID: "securityGroup", Name: "Security Group"}); res.Error != nil {
			return res.Error
		}

		if res := tx.Model(&model.Product{}).Where("product_type_id = ?", "firewall").Update("product_type_id", "securityGroup"); res.Error != nil {
			return res.Error
		}

		if res := tx.Delete(&model.ProductType{ID: "firewall"}); res.Error != nil {
			return res.Error
		}

		return nil
	},
	Rollback: func(tx *gorm.DB) error {
		log.Info().Msgf("rolling back migration 202607291200")

		if res := tx.Create(&model.ProductType{ID: "firewall", Name: "Firewall"}); res.Error != nil {
			return res.Error
		}

		if res := tx.Model(&model.Product{}).Where("product_type_id = ?", "securityGroup").Update("product_type_id", "firewall"); res.Error != nil {
			return res.Error
		}

		if res := tx.Delete(&model.ProductType{ID: "securityGroup"}); res.Error != nil {
			return res.Error
		}

		return nil
	},
}
