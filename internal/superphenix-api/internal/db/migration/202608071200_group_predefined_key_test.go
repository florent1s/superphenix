package migration

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAdoptable202608071200(t *testing.T) {
	catalog := []string{"spx_member", "IAMFullAccess", "BillingFullAccess", "ProjectBucketFullAccess"}

	tests := []struct {
		name      string
		candidate []string
		want      bool
	}{
		{
			name:      "exact match is adopted",
			candidate: []string{"spx_member", "IAMFullAccess", "BillingFullAccess", "ProjectBucketFullAccess"},
			want:      true,
		},
		{
			name:      "order does not matter",
			candidate: []string{"ProjectBucketFullAccess", "spx_member", "BillingFullAccess", "IAMFullAccess"},
			want:      true,
		},
		{
			name:      "strict subset is adopted, the catalog only ever grew",
			candidate: []string{"spx_member", "IAMFullAccess"},
			want:      true,
		},
		{
			name:      "empty group is adopted",
			candidate: []string{},
			want:      true,
		},
		{
			name:      "superset is left custom, it was hand-tuned",
			candidate: []string{"spx_member", "IAMFullAccess", "ProjectInstanceFullAccess"},
			want:      false,
		},
		{
			name:      "one unknown set is enough to keep it custom",
			candidate: []string{"SomethingElse"},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isAdoptable202608071200(tt.candidate, catalog))
		})
	}
}
