package storage

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// ErrNotFound is returned by Get when the requested key does not exist —
// a distinguishable sentinel, mirroring internal/db.ErrNotFound's shape,
// so internal/handlers/fetch.go can map it to 404 without inspecting the
// underlying AWS SDK error type itself.
var ErrNotFound = errors.New("storage: not found")

// isNotFound translates the SDK's own not-found error shapes (S3's typed
// NoSuchKey, and LocalStack's occasionally-untyped-but-code-matching
// equivalent) into a single boolean Get can act on.
func isNotFound(err error) bool {
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound":
			return true
		}
	}
	return false
}
