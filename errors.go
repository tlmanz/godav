package godav

import "fmt"

// UploadError represents errors that occur during upload
type UploadError struct {
	Op      string // Operation that failed
	Path    string // File path involved
	Err     error  // Underlying error
	Retries int    // Number of retries attempted
}

func (e *UploadError) Error() string {
	if e.Retries > 0 {
		return fmt.Sprintf("%s %s: %v (after %d retries)", e.Op, e.Path, e.Err, e.Retries)
	}
	return fmt.Sprintf("%s %s: %v", e.Op, e.Path, e.Err)
}

func (e *UploadError) Unwrap() error {
	return e.Err
}
