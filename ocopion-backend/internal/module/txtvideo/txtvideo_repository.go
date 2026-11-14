package txtvideo

import (
	"context"
	"io"
	"os"
)

// Repository owns temporary files, command execution and publication of finished videos.
type Repository interface {
	CheckDependencies() error
	Prepare(io.Reader) (string, error)
	Generate(context.Context, string, string, string, string) error
	Cleanup(string)
	Open(string) (*os.File, error)
}
