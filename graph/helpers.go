package graph

import (
	"nats-graphql/graph/model"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

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
