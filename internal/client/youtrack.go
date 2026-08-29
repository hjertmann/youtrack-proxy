// Package client provides HTTP functions for communicating with the YouTrack REST API.
// All outbound calls go through a shared http.Client with optional concurrency gating.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/semaphore"

	"github.com/hjertmann/youtrack-proxy/internal/config"
	"github.com/hjertmann/youtrack-proxy/internal/model"
)

// maxResponseBytes caps a single response body read to prevent OOM.
// ponytail: 10 MiB is generous for JSON; raise if attachments are proxied.
const maxResponseBytes = 10 << 20

// readCappedBody reads up to maxResponseBytes from r, returning an error when
// the body exceeds that limit. It reads one byte past the cap so an oversized
// response is detected rather than silently truncated.
func readCappedBody(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxResponseBytes {
		return nil, fmt.Errorf("response exceeds %d byte limit", maxResponseBytes)
	}
	return data, nil
}

// ---------------------------------------------------------------------------
// Shared HTTP client
// ---------------------------------------------------------------------------

var sharedClient *http.Client

// InitHTTPClient creates the shared HTTP client used for all outbound calls.
func InitHTTPClient(cfg config.Config) {
	sharedClient = &http.Client{Timeout: cfg.RequestTimeout}
}

// resolveClient returns the shared client, falling back to an ad-hoc client
// when InitHTTPClient has not been called (e.g. in tests).
func resolveClient(cfg *config.Config) *http.Client {
	if sharedClient != nil {
		return sharedClient
	}
	return &http.Client{Timeout: cfg.RequestTimeout}
}

// ---------------------------------------------------------------------------
// Concurrency gating
// ---------------------------------------------------------------------------

var ytSemaphore *semaphore.Weighted

// InitConcurrency configures the maximum number of concurrent outbound requests.
func InitConcurrency(n int64) {
	ytSemaphore = semaphore.NewWeighted(n)
}

// DisableConcurrency removes the concurrency gate (useful in tests).
func DisableConcurrency() {
	ytSemaphore = nil
}

// acquireSemaphore blocks until a concurrency slot is available or the queue
// timeout elapses. Returns a release function the caller must defer on success.
func acquireSemaphore(cfg *config.Config) (func(), error) {
	if ytSemaphore == nil {
		return func() {}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.QueueTimeout)
	if err := ytSemaphore.Acquire(ctx, 1); err != nil {
		cancel()
		return nil, ErrQueueTimeout
	}
	return func() {
		ytSemaphore.Release(1)
		cancel()
	}, nil
}

// ---------------------------------------------------------------------------
// Low-level helpers
// ---------------------------------------------------------------------------

// setAuth sets the Bearer token and Accept headers on an outbound request.
func setAuth(req *http.Request, token string) {
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
}

// ---------------------------------------------------------------------------
// Generic request functions
// ---------------------------------------------------------------------------

// CreateYouTrackIssue POSTs a new issue to YouTrack and returns the response.
func CreateYouTrackIssue(
	payload *model.YouTrackCreateIssueRequest,
	rctx *model.RequestContext,
	cfg *config.Config,
) (*model.YouTrackResponse, error) {
	release, err := acquireSemaphore(cfg)
	if err != nil {
		return nil, err
	}
	defer release()

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshaling issue payload: %v", err)
	}

	endpoint := strings.TrimRight(cfg.YouTrackURL, "/") + "/api/issues"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("building POST request: %v", err)
	}
	setAuth(req, rctx.YouTrackToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := resolveClient(cfg).Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing POST to YouTrack: %v", err)
	}
	defer resp.Body.Close()

	respBytes, err := readCappedBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading POST response: %v", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("YouTrack API error (status %d): %s", resp.StatusCode, string(respBytes))
	}

	var out model.YouTrackResponse
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return nil, fmt.Errorf("decoding issue response: %v", err)
	}
	return &out, nil
}

// GetFromYouTrack performs an authenticated GET against the YouTrack API.
// It returns the raw body, HTTP status, and any error. Non-2xx responses
// produce a *YouTrackError carrying the status code and body text.
func GetFromYouTrack(
	path string,
	fields string,
	queryParams map[string]string,
	rctx *model.RequestContext,
	cfg *config.Config,
) ([]byte, int, error) {
	release, err := acquireSemaphore(cfg)
	if err != nil {
		return nil, 0, err
	}
	defer release()

	u, err := url.Parse(strings.TrimRight(cfg.YouTrackURL, "/") + path)
	if err != nil {
		return nil, 0, fmt.Errorf("parsing URL: %v", err)
	}

	q := u.Query()
	if fields != "" {
		q.Set("fields", fields)
	}
	for k, v := range queryParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, 0, fmt.Errorf("building GET request: %v", err)
	}
	setAuth(req, rctx.YouTrackToken)

	resp, err := resolveClient(cfg).Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("executing GET to YouTrack: %v", err)
	}
	defer resp.Body.Close()

	respBytes, err := readCappedBody(resp.Body)
	if err != nil {
		return nil, 0, fmt.Errorf("reading GET response: %v", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode, &YouTrackError{
			StatusCode: resp.StatusCode,
			Message:    string(respBytes),
		}
	}
	return respBytes, resp.StatusCode, nil
}

// ---------------------------------------------------------------------------
// Field constants for YouTrack API queries
// ---------------------------------------------------------------------------

const (
	projectFields  = "id,name,shortName,description,leader(login,fullName,email,banned)"
	issueFields    = "idReadable,id,summary,description,created,updated,resolved,reporter(login,fullName),project(id,name,shortName),tags(name),customFields(name,value(id,name,login,fullName))"
	commentFields  = "id,author(login,fullName),text,created,updated"
	userFields     = "login,fullName,email,banned"
	activityFields = "id,timestamp,author(login,fullName),field(id,name,presentation),added(id,name),removed(id,name),$type"
)

// ---------------------------------------------------------------------------
// Typed resource fetchers
// ---------------------------------------------------------------------------

// GetProjects retrieves every project, paginating and deduplicating by ID.
func GetProjects(rctx *model.RequestContext, cfg *config.Config) ([]model.YTProject, error) {
	const pageSize = 100
	seen := make(map[string]struct{})
	var all []model.YTProject
	fetched := 0

	for skip := 0; ; skip += pageSize {
		params := map[string]string{
			"$skip": strconv.Itoa(skip),
			"$top":  strconv.Itoa(pageSize),
		}
		raw, _, err := GetFromYouTrack("/api/admin/projects", projectFields, params, rctx, cfg)
		if err != nil {
			return nil, err
		}
		var page []model.YTProject
		if err := json.Unmarshal(raw, &page); err != nil {
			return nil, fmt.Errorf("decoding projects page: %v", err)
		}
		fetched += len(page)
		for _, p := range page {
			if _, dup := seen[p.ID]; !dup {
				seen[p.ID] = struct{}{}
				all = append(all, p)
			}
		}
		if len(page) < pageSize {
			break
		}
	}

	log.Debug().Int("totalFetched", fetched).Int("unique", len(all)).Msg("Fetched all projects from YouTrack")
	return all, nil
}

// GetProject retrieves a single project by ID or short name.
func GetProject(id string, rctx *model.RequestContext, cfg *config.Config) (*model.YTProject, error) {
	raw, _, err := GetFromYouTrack("/api/admin/projects/"+url.PathEscape(id), projectFields, nil, rctx, cfg)
	if err != nil {
		return nil, err
	}
	var p model.YTProject
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("decoding project: %v", err)
	}
	return &p, nil
}

// GetIssues searches issues with an optional query and pagination.
func GetIssues(query string, skip, top int, rctx *model.RequestContext, cfg *config.Config) ([]model.YTIssue, error) {
	params := map[string]string{
		"$skip": strconv.Itoa(skip),
		"$top":  strconv.Itoa(top),
	}
	if query != "" {
		params["query"] = query
	}
	raw, _, err := GetFromYouTrack("/api/issues", issueFields, params, rctx, cfg)
	if err != nil {
		return nil, err
	}
	var issues []model.YTIssue
	if err := json.Unmarshal(raw, &issues); err != nil {
		return nil, fmt.Errorf("decoding issues: %v", err)
	}
	return issues, nil
}

// GetIssue retrieves a single issue by its readable ID.
func GetIssue(id string, rctx *model.RequestContext, cfg *config.Config) (*model.YTIssue, error) {
	raw, _, err := GetFromYouTrack("/api/issues/"+url.PathEscape(id), issueFields, nil, rctx, cfg)
	if err != nil {
		return nil, err
	}
	var issue model.YTIssue
	if err := json.Unmarshal(raw, &issue); err != nil {
		return nil, fmt.Errorf("decoding issue: %v", err)
	}
	return &issue, nil
}

// GetIssueComments retrieves comments for an issue with pagination.
func GetIssueComments(issueID string, skip, top int, rctx *model.RequestContext, cfg *config.Config) ([]model.YTComment, error) {
	params := map[string]string{
		"$skip": strconv.Itoa(skip),
		"$top":  strconv.Itoa(top),
	}
	raw, _, err := GetFromYouTrack("/api/issues/"+url.PathEscape(issueID)+"/comments", commentFields, params, rctx, cfg)
	if err != nil {
		return nil, err
	}
	var comments []model.YTComment
	if err := json.Unmarshal(raw, &comments); err != nil {
		return nil, fmt.Errorf("decoding comments: %v", err)
	}
	return comments, nil
}

// GetCurrentUser retrieves the authenticated user.
func GetCurrentUser(rctx *model.RequestContext, cfg *config.Config) (*model.YTUser, error) {
	raw, _, err := GetFromYouTrack("/api/users/me", userFields, nil, rctx, cfg)
	if err != nil {
		return nil, err
	}
	var u model.YTUser
	if err := json.Unmarshal(raw, &u); err != nil {
		return nil, fmt.Errorf("decoding current user: %v", err)
	}
	return &u, nil
}

// GetProjectCustomFields retrieves the custom field configurations for a project,
// including bundle values (allowed values for enum fields).
func GetProjectCustomFields(projectID string, rctx *model.RequestContext, cfg *config.Config) ([]model.YTProjectCustomField, error) {
	path := "/api/admin/projects/" + url.PathEscape(projectID) + "/customFields"
	raw, _, err := GetFromYouTrack(path, "id,field(name,$type),bundle(values(id,name,isResolved)),$type", nil, rctx, cfg)
	if err != nil {
		return nil, err
	}
	var fields []model.YTProjectCustomField
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("decoding project custom fields: %v", err)
	}
	return fields, nil
}

// SearchUsers finds users matching a query string.
func SearchUsers(query string, rctx *model.RequestContext, cfg *config.Config) ([]model.YTUser, error) {
	params := map[string]string{}
	if query != "" {
		params["query"] = query
	}
	raw, _, err := GetFromYouTrack("/api/users", userFields, params, rctx, cfg)
	if err != nil {
		return nil, err
	}
	var users []model.YTUser
	if err := json.Unmarshal(raw, &users); err != nil {
		return nil, fmt.Errorf("decoding users: %v", err)
	}
	return users, nil
}

// CountIssues returns the total number of issues matching a query by paging
// through lightweight ID-only responses.
func CountIssues(query string, rctx *model.RequestContext, cfg *config.Config) (int, error) {
	const batch = 10000
	total := 0
	for skip := 0; ; skip += batch {
		params := map[string]string{
			"$skip": strconv.Itoa(skip),
			"$top":  strconv.Itoa(batch),
		}
		if query != "" {
			params["query"] = query
		}
		raw, _, err := GetFromYouTrack("/api/issues", "idReadable", params, rctx, cfg)
		if err != nil {
			return 0, fmt.Errorf("counting issues: %w", err)
		}
		var ids []struct {
			IDReadable string `json:"idReadable"`
		}
		if err := json.Unmarshal(raw, &ids); err != nil {
			return 0, fmt.Errorf("parsing count response: %w", err)
		}
		total += len(ids)
		if len(ids) < batch {
			break
		}
	}
	return total, nil
}

// GetIssueActivities retrieves the change history for an issue (CustomFieldCategory only).
func GetIssueActivities(issueID string, skip, top int, rctx *model.RequestContext, cfg *config.Config) ([]model.YTActivityItem, error) {
	params := map[string]string{
		"$skip":      strconv.Itoa(skip),
		"$top":       strconv.Itoa(top),
		"categories": "CustomFieldCategory",
	}
	raw, _, err := GetFromYouTrack("/api/issues/"+url.PathEscape(issueID)+"/activities", activityFields, params, rctx, cfg)
	if err != nil {
		return nil, err
	}
	var items []model.YTActivityItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("decoding activities: %v", err)
	}
	return items, nil
}

// GetAllProjectCustomFields fetches Type and State bundle values across all
// projects. Per-project failures are logged and skipped.
func GetAllProjectCustomFields(rctx *model.RequestContext, cfg *config.Config) ([]model.YTProjectCustomField, error) {
	projects, err := GetProjects(rctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("fetching projects for custom fields: %w", err)
	}
	var result []model.YTProjectCustomField
	for _, p := range projects {
		fields, err := GetProjectCustomFields(p.ID, rctx, cfg)
		if err != nil {
			log.Warn().Str("project", p.ShortName).Err(err).Msg("skipping project custom fields")
			continue
		}
		for _, f := range fields {
			if f.Field.Name == "Type" || f.Field.Name == "State" {
				result = append(result, f)
			}
		}
	}
	return result, nil
}
