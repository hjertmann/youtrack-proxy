---
inclusion: fileMatch
fileMatchPattern: "internal/service/converter*.go,internal/handler/*.go,internal/model/jira_read.go,internal/client/youtrack.go,internal/model/youtrack_read.go"
---

# Jira REST API Response Compliance

## Non-Optional Fields Must Always Have Values

When building Jira-compatible JSON responses, every field that a Jira client may treat as non-nullable **must** have a value. This applies to all response types returned by this proxy.

### Rules

1. **Always include `id` fields** — Every Jira DTO object (issue types, priorities, statuses, users, projects, fields) expects an `id` property. For entities with YouTrack string IDs (e.g. `"0-4"`, `"69-774"`), use `idmap.Encode` to produce a deterministic numeric int64 (see Pattern Reference below). For fields without a YouTrack ID, fall back to `strings.ToLower(strings.ReplaceAll(name, " ", "-"))`.

2. **Never omit required struct fields** — If a Jira client DTO declares a field as non-nullable (e.g., Kotlin `val id: String`), the proxy must always populate it. Known non-nullable fields on Jira DTOs include:
   - `id` on issue types, priorities, statuses, and projects
   - `key` on issues and projects
   - `name` on named fields (issue type, priority, status)
   - `summary` on issues
   - `accountId` on user objects

3. **Use fallback defaults for missing data** — When YouTrack data is absent or null for a required field:
   - For `id`: prefer `idmap.Encode(ytID)` → `idmap.FormatID(numID)`. If encoding fails, fall back to the raw YT ID with a warning log. If no YT ID exists at all, derive from name via lowercase + hyphen replacement.
   - For `name`: use `"Unknown"` as a fallback
   - For `key`: use the readable ID or short name
   - For `accountId`: use the login field

4. **Apply to `allowedValues` entries** — Each entry in an `allowedValues` array must include all fields the client expects (minimally `id` and `name`). A map like `{"name": "Bug"}` is insufficient — it must be `{"id": "bug", "name": "Bug"}`.

5. **Include `issueTypes` on project responses** — The IntelliJ plugin expects project objects to have an `issueTypes` array with entries containing non-null `id`, `name`, `self`, `description`, and `subtask` fields.

6. **Validate during code review** — When adding or modifying response-building functions, verify that:
   - All map entries in `allowedValues` include `"id"` and `"name"`
   - All `JiraNamedField` structs have non-empty `ID` and `Name`
   - All `JiraProject` structs have non-empty `Id`, `Key`, `Name`, and `IssueTypes`
   - All `JiraUserResponse` structs have non-empty `AccountId`, `Key`, and `Name`
   - All `JiraIssueType` entries have non-empty `ID` and `Name`

### Pattern Reference

**Primary: Bitfield encoding via `internal/idmap`** (see `handler/pickers.go`, `handler/boards.go`, `service/converter_read.go`):

```go
numID, err := idmap.Encode(ytStringID) // "0-4" → 4, "69-774" → 4899942093438774
strID := idmap.FormatID(numID)          // int64 → decimal string
ytID, ok := idmap.Decode(numID)         // reverse: int64 → "0-4"
```

This produces deterministic, reversible numeric IDs that Jira clients expect. Encoding errors should return 500 in handlers or fall back to the raw YT ID with a warning in converters.

**Fallback: Synthetic slug IDs** (used when no YT string ID is available, e.g. `bundleValuesToAllowed`):

```go
id := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
```

This produces stable string IDs: `"In Progress"` → `"in-progress"`, `"Bug"` → `"bug"`.

### Why This Matters

The IntelliJ Jira plugin (and other Jira clients) uses Kotlin data classes with non-nullable constructor parameters. If any expected field is missing from the JSON, Jackson's `KotlinValueInstantiator` throws `MissingKotlinParameterException` and the client crashes. This proxy must guarantee complete responses.

---

## YouTrack API Field Requests Must Match Model Needs

When fetching data from YouTrack, the `fields` query parameter controls which properties are returned. If the proxy model expects a field but the API query doesn't request it, the field will be empty/zero-valued and silently break downstream.

### Rules

1. **Keep `fields` query params in sync with Go struct fields** — If a model struct (in `internal/model/youtrack_read.go`) has a JSON-tagged field, the corresponding YouTrack API query must request that field. For example, if `YTBundleValue` has `ID string \`json:"id"\``, the bundle values query must include `id` in its fields param: `bundle(values(id,name))`.

2. **When adding a field to a YouTrack model struct**, always update the corresponding `fields` constant or query string in `internal/client/youtrack.go`.

3. **When reusing converters across contexts**, ensure all contexts request the same fields. For example, if `ConvertYTUserToJira` populates `AccountId` from `Login` and `EmailAddress` from `Email`, then every API query that returns users must request `login,fullName,email,banned` — not just `login,fullName`.

4. **Established field constants** (in `internal/client/youtrack.go`):
   - `projectFields` — used for project list and single project endpoints
   - `issueFields` — used for issue search and single issue endpoints
   - `commentFields` — used for issue comments endpoint
   - `userFields` — used for user/myself and user search endpoints
   - `activityFields` — used for issue changelog/activity endpoints

### Checklist When Adding a New Response Field

1. Add the field to the YouTrack model struct (`internal/model/youtrack_read.go`)
2. Update the `fields` query constant in `internal/client/youtrack.go`
3. Add the field to the Jira response model (`internal/model/jira_read.go` or `models.go`)
4. Map it in the converter (`internal/service/converter_read.go`)
5. If the field is an ID visible to Jira clients, use `idmap.Encode` / `idmap.FormatID`
6. Ensure fallback/default value if the field can be absent
7. Run `go test ./...` — tests should catch missing mappings
