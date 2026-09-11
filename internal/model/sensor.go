package model

import (
	"fmt"
	"time"
)

// SensorStatus records the last reported health of a named sensor. Degraded
// reports carry the observed failure class, never fabricated coverage.
type SensorStatus struct {
	Version int       `json:"version"`
	Sensor  string    `json:"sensor"`
	Status  string    `json:"status"` // healthy | degraded
	Reason  string    `json:"reason,omitempty"`
	At      time.Time `json:"at"`
}

func (s SensorStatus) Validate() error {
	if s.Version != 1 || !workspaceText(s.Sensor, 128) || !Contains([]string{"healthy", "degraded"}, s.Status) || (s.Status == "degraded" && (len(s.Reason) < 1 || len(s.Reason) > 256)) || s.At.IsZero() {
		return fmt.Errorf("invalid sensor status")
	}
	return nil
}
