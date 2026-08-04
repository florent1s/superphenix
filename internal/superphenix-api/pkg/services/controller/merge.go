package controller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/super-phenix/superphenix/internal/superphenix-api/internal/db/model"
	logger "github.com/super-phenix/superphenix/pkg/utils/log"
)

// MergeOutcome tells the caller which sources contributed to the merged product response.
type MergeOutcome int

const (
	// MergeNotFound means neither the AZ controller nor the database knows the resource.
	// Callers answer 404.
	MergeNotFound MergeOutcome = iota
	// MergeDbOnly means the database row is the only trustworthy source: the AZ controller is
	// unreachable, answered 404, returned an unusable body, or returned a resource that is not
	// the one we asked for. No AZ payload is attached.
	MergeDbOnly
	// MergeAzOnly means no database row exists (gitops-managed or out-of-band resource) and the
	// response is built from the AZ payload alone.
	MergeAzOnly
	// MergeBoth means the database row and the AZ resource agree on the resource identity.
	MergeBoth
)

// ResolveProductResponse merges the AZ controller response with the product row read from the
// database and returns the shared ProductResponse envelope, the decoded AZ body and the outcome
// of the merge.
//
// resp may be nil when the proxy call failed. The returned map is nil unless the AZ payload is
// usable, so callers can attach their product-specific fields with a single `if az != nil` guard.
//
// When the AZ answers with a resource whose local ID does not match the database row, the
// database is trusted: this happens when the AZ resource was never created (a failed GitOps sync,
// for instance), and returning the database identity keeps the resource manageable — editable,
// deletable and linkable to Argo CD — instead of surfacing an empty product.
func ResolveProductResponse(ctx context.Context, azCode string, resp *http.Response, dbProduct model.Product, dbErr error) (ProductResponse, map[string]interface{}, MergeOutcome) {
	log := logger.GetLogger(ctx)
	azResult := decodeAZBody(ctx, resp)
	hasDb := dbErr == nil

	switch {
	case azResult == nil && !hasDb:
		return ProductResponse{}, nil, MergeNotFound

	case azResult == nil:
		return dbProductResponse(dbProduct, azCode), nil, MergeDbOnly

	case !hasDb:
		log.Info().Err(dbErr).Str("resourceEId", stringField(azResult, "eid")).Msg("Resource not found in DB")

		return ProductResponse{
			ID:          stringField(azResult, "id"),
			EId:         stringField(azResult, "eid"),
			ProductName: stringField(azResult, "productName"),
			CodeAZ:      azCode,
			Gitops:      stringField(azResult, "gitops"),
		}, azResult, MergeAzOnly

	case dbProduct.ID.String() == stringField(azResult, "id"):
		return ProductResponse{
			ID:            dbProduct.ID.String(),
			EId:           stringField(azResult, "eid"),
			ProductName:   dbProduct.ProductName,
			CodeAZ:        azCode,
			ProductTypeId: dbProduct.ProductTypeId,
			Gitops:        stringField(azResult, "gitops"),
		}, azResult, MergeBoth

	default:
		log.Warn().
			Str("eid", dbProduct.EffectiveID).
			Str("dbId", dbProduct.ID.String()).
			Str("azId", stringField(azResult, "id")).
			Str("az", azCode).
			Msg("AZ resource does not match the database row, falling back to database information")

		return dbProductResponse(dbProduct, azCode), nil, MergeDbOnly
	}
}

// dbProductResponse builds the response from the database row alone. Such a resource is never
// gitops-managed: it was created through the API.
func dbProductResponse(dbProduct model.Product, azCode string) ProductResponse {
	return ProductResponse{
		ID:            dbProduct.ID.String(),
		EId:           dbProduct.EffectiveID,
		ProductName:   dbProduct.ProductName,
		CodeAZ:        azCode,
		ProductTypeId: dbProduct.ProductTypeId,
		Gitops:        "false",
	}
}

// decodeAZBody returns the AZ controller payload, or nil when there is nothing usable to read:
// no response at all, a status other than 200, or a body that is not a JSON object.
func decodeAZBody(ctx context.Context, resp *http.Response) map[string]interface{} {
	if resp == nil || resp.StatusCode != http.StatusOK {
		return nil
	}

	log := logger.GetLogger(ctx)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Error reading response body")
		return nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal the AZ controller response")
		return nil
	}

	return result
}

// stringField reads a string field from a decoded JSON object, returning an empty string when the
// key is absent or holds another type.
func stringField(result map[string]interface{}, key string) string {
	value, _ := result[key].(string)
	return value
}
