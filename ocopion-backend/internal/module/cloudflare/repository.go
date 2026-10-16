package cloudflare

import "context"

type Repository interface {
	Measure(context.Context) ([]Candidate, error)
	Save([]string) error
}
