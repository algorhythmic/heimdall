package store

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/conversation"
	"heimdall/internal/model"
	"strings"
	"time"
)

func applySensor(st *model.State, e Event) error {
	var p model.SensorStatus
	if err := model.StrictJSON(e.Payload, &p); err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if !strings.HasPrefix(e.Actor, "observer:") || e.EntityID != p.Sensor || !strings.HasPrefix(e.CommandID, "sensor-") || !e.TS.Equal(p.At) {
		return fmt.Errorf("invalid sensor event provenance")
	}
	verb := map[string]string{"sensor.degraded": "degraded", "sensor.recovered": "healthy"}[e.Subject+"."+e.Verb]
	if p.Status != verb {
		return fmt.Errorf("sensor status/verb mismatch")
	}
	old, ok := st.SensorHealth[p.Sensor]
	if ok && !p.At.After(old.At) {
		return fmt.Errorf("nonmonotonic sensor report")
	}
	st.SensorHealth[p.Sensor] = p
	return nil
}

// ReportSensor journals a health transition for a named sensor. Identical
// repeated reports dedupe through the content-derived command ID.
func (s *Store) ReportSensor(ctx context.Context, sensor, status, reason, actor string, now time.Time) (json.RawMessage, error) {
	if !strings.HasPrefix(actor, "observer:") {
		return nil, fmt.Errorf("sensor reports require observer authority")
	}
	p := model.SensorStatus{Version: 1, Sensor: sensor, Status: status, Reason: reason, At: now.UTC()}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	verb := "sensor.recovered"
	if status == "degraded" {
		verb = "sensor.degraded"
	}
	raw, _ := json.Marshal(p)
	id := "sensor-" + conversation.Digest([]byte(conversation.Identity("heimdall-sensor", sensor, verb, string(raw))))
	return s.Transact(ctx, id, actor, raw, now, func(st model.State) (Change, error) {
		return Change{Revision: st.Revision, Events: []Pending{{"sensor", map[bool]string{true: "degraded", false: "recovered"}[status == "degraded"], sensor, p}}}, nil
	})
}
