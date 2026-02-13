package graph

import "time"

// parseRFC3339 parses a time string in RFC3339 or RFC3339Nano format.
func parseRFC3339(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
