package ingestion

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestCannotSetNewValuesAfterStoppingStarted(t *testing.T) {

	sah := NewStopAtHeight()

	// first update is always successful
	oldSet, _, _, err := sah.Set(21, false)
	require.NoError(t, err)
	require.False(t, oldSet)

	sah.Try(func(height uint64, crash bool) bool {
		return false // no stopping has started
	})

	oldSet, _, _, err = sah.Set(37, false)
	require.NoError(t, err)
	require.True(t, oldSet)

	sah.Try(func(height uint64, crash bool) bool {
		return true
	})

	_, _, _, err = sah.Set(2137, false)
	require.Error(t, err)

}
