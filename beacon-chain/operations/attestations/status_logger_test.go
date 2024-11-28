package attestations

import (
	"sync"
	"testing"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/testing/require"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
)

func TestIncVerifiedAttestations(t *testing.T) {
	tests := []struct {
		name          string
		incrementBy   int
		expectedCount int
		concurrent    bool
	}{
		{
			name:          "Single Increment",
			incrementBy:   1,
			expectedCount: 1,
			concurrent:    false,
		},
		{
			name:          "Multiple Increments",
			incrementBy:   3,
			expectedCount: 3,
			concurrent:    false,
		},
		{
			name:          "Concurrent Increments",
			incrementBy:   100,
			expectedCount: 100,
			concurrent:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &StatusLogger{
				mu:                 &sync.Mutex{},
				genesisTime:        0,
				failedAttestations: make(map[string]int),
			}
			errReason := "test"

			if tt.concurrent {
				var wg sync.WaitGroup
				for i := 0; i < tt.incrementBy; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						st.IncVerifiedAttestations()
						st.AddFailedAttestation(errReason)
					}()
				}
				wg.Wait()
			} else {
				for i := 0; i < tt.incrementBy; i++ {
					st.IncVerifiedAttestations()
					st.AddFailedAttestation(errReason)
				}
			}

			require.Equal(t, tt.expectedCount, st.VerifiedAttestations())
			require.Equal(t, tt.expectedCount, st.FailedAttestations()[errReason])
		})
	}
}

func TestReset(t *testing.T) {
	st := &StatusLogger{
		mu:                 &sync.Mutex{},
		genesisTime:        0,
		failedAttestations: make(map[string]int),
	}
	errorReason := "test"

	st.IncVerifiedAttestations()
	st.AddFailedAttestation(errorReason)

	require.Equal(t, 1, st.VerifiedAttestations())
	require.Equal(t, 1, st.FailedAttestations()[errorReason])

	st.Reset()

	require.Equal(t, 0, st.VerifiedAttestations())
	require.Equal(t, 0, len(st.FailedAttestations()))
}

func TestLogOnCloseToNextEpoch(t *testing.T) {
	st := &StatusLogger{
		mu:          &sync.Mutex{},
		genesisTime: uint64(prysmTime.Now().Unix()),
	}

	if st.isNextSlotNewEpoch() {
		t.Error("expected next slot to be in the same epoch")
	}

	// Go forward until one slot before the next epoch
	st.genesisTime -= uint64(params.BeaconConfig().SlotsPerEpoch.Mul(
		params.BeaconConfig().SecondsPerSlot)) - params.BeaconConfig().SecondsPerSlot

	if !st.isNextSlotNewEpoch() {
		t.Error("expected next slot to be in the same next epoch")
	}
}
