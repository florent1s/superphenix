package kaas

import (
	"testing"

	"github.com/super-phenix/superphenix/internal/superphenix-api/internal/db/model"
	"github.com/super-phenix/superphenix/internal/superphenix-api/pkg/config"

	"github.com/google/uuid"
)

func TestCombineListResult(t *testing.T) {
	k := &config.Global.ProductsConfig.ArgoApp.Kubernetes
	prevRepo, prevVersions := k.Repo, k.KubeVersions
	t.Cleanup(func() { k.Repo, k.KubeVersions = prevRepo, prevVersions })
	k.Repo = config.RepoArgoAppConfig{RepoURL: "ghcr.io/super-phenix/charts", Chart: "sfs-kaas", TargetRevision: "0.7.1"}
	k.KubeVersions = []config.KubeVersionConfig{{Version: "v1.36.3"}}

	product := model.Product{EffectiveID: "spx-eid", ProductName: "cluster", CodeAZ: "az1"}
	product.ID = uuid.New()
	azEntry := func(chart, kubeVersion string) map[string]interface{} {
		entry := map[string]interface{}{"id": product.ID.String(), "eid": product.EffectiveID, "gitops": "false", "cluster": nil}
		if chart != "" {
			entry["chart"] = chart
		}
		if kubeVersion != "" {
			entry["kubeVersion"] = kubeVersion
		}
		return entry
	}
	flag := func(b bool) *bool { return &b }

	tests := []struct {
		name   string
		az     map[string][]interface{}
		want *bool
	}{
		{name: "up to date", az: map[string][]interface{}{"az1": {azEntry("sfs-kaas-0.7.1", "v1.36.3")}}, want: flag(false)},
		{name: "outdated", az: map[string][]interface{}{"az1": {azEntry("sfs-kaas-0.3.8", "v1.36.3")}}, want: flag(true)},
		{name: "controller without chart", az: map[string][]interface{}{"az1": {azEntry("", "")}}, want: nil},
		{name: "db only", az: map[string][]interface{}{}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			check := map[uuid.UUID]bool{product.ID: false}
			got := combineListResult(tt.az, []model.Product{product}, check)
			if len(got) != 1 {
				t.Fatalf("combineListResult() returned %d entries, want 1", len(got))
			}
			switch {
			case tt.want == nil && got[0].Outdated != nil:
				t.Errorf("Outdated = %v, want nil", *got[0].Outdated)
			case tt.want != nil && (got[0].Outdated == nil || *got[0].Outdated != *tt.want):
				t.Errorf("Outdated = %v, want %v", got[0].Outdated, *tt.want)
			}
		})
	}
}
