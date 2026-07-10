package chatd

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

func TestAdvisorConfigStorage(t *testing.T) {
	t.Parallel()

	t.Run("RoundTripCurrentVersion", func(t *testing.T) {
		t.Parallel()
		want := codersdk.AdvisorConfig{
			MaxUsesPerRun:   3,
			MaxOutputTokens: 1024,
			ModelConfigID:   uuid.New(),
			ReasoningEffort: ptr.Ref(codersdk.ChatModelReasoningEffortHigh),
		}

		encoded, err := EncodeAdvisorConfig(want)
		require.NoError(t, err)
		require.Contains(t, string(encoded), `"_version":1`)

		got, err := DecodeAdvisorConfig(encoded)
		require.NoError(t, err)
		require.Equal(t, want, got)
	})

	t.Run("IgnoresUnversionedReasoningEffort", func(t *testing.T) {
		t.Parallel()
		modelConfigID := uuid.New()
		raw := []byte(`{"model_config_id":"` + modelConfigID.String() + `","reasoning_effort":"high"}`)

		got, err := DecodeAdvisorConfig(raw)
		require.NoError(t, err)
		require.Equal(t, modelConfigID, got.ModelConfigID)
		require.Nil(t, got.ReasoningEffort)
	})
}
