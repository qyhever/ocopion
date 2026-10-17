package cloudflare

import "context"

type Repository interface {
	Measure(context.Context) ([]Candidate, error)
	Save([]string) error
	ReadClashTemplate() ([]byte, error)
	SaveClash([]byte) error
	UploadClash(context.Context) error
}
