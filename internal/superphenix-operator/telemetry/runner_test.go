package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunner_NeedLeaderElection(t *testing.T) {
	r := &Runner{}
	assert.True(t, r.NeedLeaderElection(), "Runner should require leader election")
}
