package graph

import (
	"encoding/json"
	"fmt"
	"nats-graphql/graph/model"
	"sort"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// parseHeaders parses a JSON string into nats.Header.
// Accepts {"key": "value"} or {"key": ["v1", "v2"]} format.
func parseHeaders(jsonStr string) (nats.Header, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("invalid headers JSON: %w", err)
	}

	h := make(nats.Header)
	for k, v := range raw {
		switch val := v.(type) {
		case string:
			h.Set(k, val)
		case []interface{}:
			for _, item := range val {
				s, ok := item.(string)
				if !ok {
					return nil, fmt.Errorf("header %q: array values must be strings", k)
				}
				h.Add(k, s)
			}
		default:
			return nil, fmt.Errorf("header %q: value must be a string or array of strings", k)
		}
	}
	return h, nil
}

// mapHeaders converts nats.Header to GraphQL HeaderEntry slice.
// Returns nil if there are no headers (so the field is null in the response).
func mapHeaders(h nats.Header) []*model.HeaderEntry {
	if len(h) == 0 {
		return nil
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	result := make([]*model.HeaderEntry, 0, len(keys))
	for _, k := range keys {
		result = append(result, &model.HeaderEntry{
			Key:    k,
			Values: h.Values(k),
		})
	}
	return result
}

// parseRFC3339 parses a time string in RFC3339 or RFC3339Nano format.
func parseRFC3339(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

// mapConsumerInfo converts JetStream ConsumerInfo to GraphQL model.
func mapConsumerInfo(ci *jetstream.ConsumerInfo) *model.ConsumerInfo {
	result := &model.ConsumerInfo{
		Stream:         ci.Stream,
		Name:           ci.Name,
		Created:        ci.Created.Format(time.RFC3339),
		DeliverPolicy:  ci.Config.DeliverPolicy.String(),
		AckPolicy:      ci.Config.AckPolicy.String(),
		AckWait:        int(ci.Config.AckWait),
		MaxDeliver:     ci.Config.MaxDeliver,
		MaxAckPending:  ci.Config.MaxAckPending,
		Replicas:       ci.Config.Replicas,
		NumAckPending:  ci.NumAckPending,
		NumRedelivered: ci.NumRedelivered,
		NumWaiting:     ci.NumWaiting,
		NumPending:     int(ci.NumPending),
		Paused:         ci.Paused,
	}

	if ci.Config.Description != "" {
		desc := ci.Config.Description
		result.Description = &desc
	}
	if ci.Config.Durable != "" {
		d := ci.Config.Durable
		result.DurableName = &d
	}
	if ci.Config.FilterSubject != "" {
		fs := ci.Config.FilterSubject
		result.FilterSubject = &fs
	}
	if len(ci.Config.FilterSubjects) > 0 {
		result.FilterSubjects = ci.Config.FilterSubjects
	}
	if ci.PauseRemaining > 0 {
		pr := int(ci.PauseRemaining)
		result.PauseRemaining = &pr
	}

	return result
}

// mapSources converts JetStream StreamSourceInfo to GraphQL model.
// Returns nil if no sources are present (so the field is null in the response).
func mapSources(sources []*jetstream.StreamSourceInfo) []*model.StreamSourceInfo {
	if len(sources) == 0 {
		return nil
	}
	result := make([]*model.StreamSourceInfo, len(sources))
	for i, src := range sources {
		s := &model.StreamSourceInfo{
			Name:   src.Name,
			Lag:    int(src.Lag),
			Active: int(src.Active),
		}
		if src.FilterSubject != "" {
			fs := src.FilterSubject
			s.FilterSubject = &fs
		}
		result[i] = s
	}
	return result
}
