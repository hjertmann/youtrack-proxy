package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/hjertmann/youtrack-proxy/internal/model"
)

// systemFields contains the static set of Jira-compatible field definitions
// for all system fields the proxy exposes in issue responses.
var systemFields = []model.JiraField{
	{
		ID:          "summary",
		Key:         "summary",
		Name:        "Summary",
		Custom:      false,
		Orderable:   true,
		Navigable:   true,
		Searchable:  true,
		ClauseNames: []string{"summary"},
		Schema:      &model.JiraFieldSchema{Type: "string", System: "summary"},
	},
	{
		ID:          "description",
		Key:         "description",
		Name:        "Description",
		Custom:      false,
		Orderable:   true,
		Navigable:   true,
		Searchable:  true,
		ClauseNames: []string{"description"},
		Schema:      &model.JiraFieldSchema{Type: "string", System: "description"},
	},
	{
		ID:          "issuetype",
		Key:         "issuetype",
		Name:        "Issue Type",
		Custom:      false,
		Orderable:   true,
		Navigable:   true,
		Searchable:  true,
		ClauseNames: []string{"issuetype", "type"},
		Schema:      &model.JiraFieldSchema{Type: "issuetype", System: "issuetype"},
	},
	{
		ID:          "priority",
		Key:         "priority",
		Name:        "Priority",
		Custom:      false,
		Orderable:   true,
		Navigable:   true,
		Searchable:  true,
		ClauseNames: []string{"priority"},
		Schema:      &model.JiraFieldSchema{Type: "priority", System: "priority"},
	},
	{
		ID:          "status",
		Key:         "status",
		Name:        "Status",
		Custom:      false,
		Orderable:   true,
		Navigable:   true,
		Searchable:  true,
		ClauseNames: []string{"status"},
		Schema:      &model.JiraFieldSchema{Type: "status", System: "status"},
	},
	{
		ID:          "assignee",
		Key:         "assignee",
		Name:        "Assignee",
		Custom:      false,
		Orderable:   true,
		Navigable:   true,
		Searchable:  true,
		ClauseNames: []string{"assignee"},
		Schema:      &model.JiraFieldSchema{Type: "user", System: "assignee"},
	},
	{
		ID:          "reporter",
		Key:         "reporter",
		Name:        "Reporter",
		Custom:      false,
		Orderable:   true,
		Navigable:   true,
		Searchable:  true,
		ClauseNames: []string{"reporter"},
		Schema:      &model.JiraFieldSchema{Type: "user", System: "reporter"},
	},
	{
		ID:          "project",
		Key:         "project",
		Name:        "Project",
		Custom:      false,
		Orderable:   true,
		Navigable:   true,
		Searchable:  true,
		ClauseNames: []string{"project"},
		Schema:      &model.JiraFieldSchema{Type: "project", System: "project"},
	},
	{
		ID:          "created",
		Key:         "created",
		Name:        "Created",
		Custom:      false,
		Orderable:   true,
		Navigable:   true,
		Searchable:  true,
		ClauseNames: []string{"created", "createdDate"},
		Schema:      &model.JiraFieldSchema{Type: "datetime", System: "created"},
	},
	{
		ID:          "updated",
		Key:         "updated",
		Name:        "Updated",
		Custom:      false,
		Orderable:   true,
		Navigable:   true,
		Searchable:  true,
		ClauseNames: []string{"updated", "updatedDate"},
		Schema:      &model.JiraFieldSchema{Type: "datetime", System: "updated"},
	},
}

// HandleListFields returns Jira-compatible field metadata for all system fields
// the proxy supports. This is a static response with no upstream API calls.
func HandleListFields(c echo.Context) error {
	return c.JSON(http.StatusOK, systemFields)
}
