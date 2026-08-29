package service

import (
	"errors"

	"github.com/hjertmann/youtrack-proxy/internal/client"
	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/idmap"
	"github.com/hjertmann/youtrack-proxy/internal/model"
)

var (
	ErrFilterNotFound   = errors.New("filter not found")
	ErrInvalidFilterJQL = errors.New("resolved filter JQL is invalid")
)

// ResolveFilterProject decodes a numeric filter ID back to a YouTrack project ID,
// fetches the corresponding project, and returns its short name.
func ResolveFilterProject(
	filterID int64,
	requestCtx *model.RequestContext,
	cfg *config.Config,
) (string, error) {
	ytProjectID, ok := idmap.Decode(filterID)
	if !ok {
		return "", ErrFilterNotFound
	}

	project, err := client.GetProject(ytProjectID, requestCtx, cfg)
	if err != nil {
		return "", err
	}

	return project.ShortName, nil
}
