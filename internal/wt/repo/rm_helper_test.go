package repo

import (
	"io"

	"wt/internal/wt/core"
)

// Rm resolves a repository by name and removes it, as the tests exercise name
// validation together with the removal guards.
func Rm(out io.Writer, repoName string, opts RmOptions) error {
	container, err := core.FindContainer(repoName)
	if err != nil {
		return err
	}
	return RmAt(out, container, opts)
}
