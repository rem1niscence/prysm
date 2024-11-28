package attestations

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prysmaticlabs/prysm/v5/config/params"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	prysmTime "github.com/prysmaticlabs/prysm/v5/time"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/sirupsen/logrus"
)

// StatusLogger tracks and logs the status of received attestations in the beacon chain.
type StatusLogger struct {
	mu                   *sync.Mutex
	genesisTime          uint64
	verifiedAttestations int
	failedAttestations   map[string]int
}

// NewStatusLogger creates a new instance of AttestationLogger with the provided genesis time.
func NewStatusLogger(genesisTime uint64) *StatusLogger {
	return &StatusLogger{
		genesisTime:        genesisTime,
		mu:                 &sync.Mutex{},
		failedAttestations: make(map[string]int),
	}
}

// IncVerifiedAttestations increments the count of verified attestations.
func (a *StatusLogger) IncVerifiedAttestations() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.verifiedAttestations++
}

// VerifiedAttestations returns the count of verified attestations.
func (a *StatusLogger) VerifiedAttestations() int {
	return a.verifiedAttestations
}

// AddFailedAttestation increments the count of failed attestations for the given key.
func (a *StatusLogger) AddFailedAttestation(reason string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failedAttestations[reason]++
}

// FailedAttestations returns the failed attestations stored.
func (a *StatusLogger) FailedAttestations() map[string]int {
	copy := make(map[string]int)
	for k, v := range a.failedAttestations {
		copy[k] = v
	}
	return copy
}

// Reset resets the attestation tracking information.
func (a *StatusLogger) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.verifiedAttestations = 0
	a.failedAttestations = make(map[string]int)
}

// LogAttestationStatus logs the number of verified and failed attestations.
func (a *StatusLogger) LogAttestationStatus() {
	fields := logrus.Fields{
		"successfulAttestations": a.VerifiedAttestations(),
	}
	for failures, count := range a.failedAttestations {
		key := fmt.Sprint("failure_", failures)
		fields[key] = count
	}
	log.Info("Attestations Summary")
}

// LogStatus periodically logs the attestation status of the beacon chain.
// The attestations status is logged one second before the start of a new epoch.
func (a *StatusLogger) LogStatus(ctx context.Context) {
	intervals := []time.Duration{
		time.Duration(params.BeaconConfig().SecondsPerSlot-1) * time.Second,
	}
	ticker := slots.NewSlotTickerWithIntervals(prysmTime.Now(), intervals)
	go func() {
		for {
			select {
			case <-ticker.C():
				if a.isNextSlotNewEpoch() {
					a.logAndReset()
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// isNextSlotNewEpoch checks whether the next slot will start a new epoch.
func (a *StatusLogger) isNextSlotNewEpoch() bool {
	now := uint64(prysmTime.Now().Unix())

	currentSlot := primitives.Slot((now - a.genesisTime) / params.BeaconConfig().SecondsPerSlot)
	nextSlot := currentSlot + 1

	currentSlotEpoch := slots.ToEpoch(currentSlot)
	nextSlotEpoch := slots.ToEpoch(nextSlot)

	fmt.Println()
	fmt.Println("CURRENT SLOT", currentSlot)
	fmt.Println("NEXT SLOT", nextSlot)
	fmt.Println("CURRENT EPOCH", currentSlotEpoch)
	fmt.Println("NEXT SLOT EPOCH", nextSlotEpoch)
	fmt.Println("RESULT", nextSlotEpoch > currentSlotEpoch)

	return nextSlotEpoch > currentSlotEpoch
}

// logAndReset logs the current attestation status and then resets the attestation counters and failure mappings.
func (a *StatusLogger) logAndReset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.LogAttestationStatus()
	a.Reset()
}
