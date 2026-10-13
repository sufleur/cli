package cli

import (
	"errors"
	"fmt"

	"github.com/Masterminds/semver/v3"

	"github.com/sufleur/cli/internal/userapi"
)

const publishSettingNote = `Publishing from the CLI works only when a workspace owner or admin has turned on
"Allow CLI and agent publishing" in the web app (Settings → Publish rules); it is
off by default and cannot be changed from the CLI. <version> must be a plain
x.y.z semantic version greater than the latest published one.`

// codedError is an error that also carries a stable machine-readable code,
// surfaced as the "code" key of --json error output.
type codedError struct {
	message string
	code    string
}

func (e *codedError) Error() string     { return e.message }
func (e *codedError) ErrorCode() string { return e.code }

func parsePublishVersion(arg string) (string, error) {
	v, err := semver.StrictNewVersion(arg)
	if err != nil || v.Prerelease() != "" || v.Metadata() != "" {
		return "", fmt.Errorf("version %q must be a plain x.y.z semantic version (for example 1.2.0)", arg)
	}
	return v.String(), nil
}

func mapPublishError(err error, workspace string) error {
	var gqlErr *userapi.GraphQLError
	if errors.As(err, &gqlErr) && gqlErr.Code == userapi.CodeCLIPublishingDisabled {
		return &codedError{
			message: fmt.Sprintf(`CLI publishing is disabled for workspace @%s. A workspace owner or admin must turn on "Allow CLI and agent publishing" in the web app (Settings → Publish rules). This setting cannot be changed from the CLI.`, workspace),
			code:    userapi.CodeCLIPublishingDisabled,
		}
	}
	return mapBearer(err)
}
