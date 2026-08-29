package service

import (
	"testing"

	"github.com/hjertmann/youtrack-proxy/internal/model"
	"pgregory.net/rapid"
)

// TestPropertyPreservationConvertYTUserToJira validates that ConvertYTUserToJira
// preserves the mapping logic: login→key/name, Name→displayName, email→emailAddress, !banned→active.
// This test captures baseline behavior BEFORE the field name fix to ensure no regressions.
//
// **Validates: Requirements 3.1, 3.3**
func TestPropertyPreservationConvertYTUserToJira(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate random YTUser with varied inputs including edge cases
		// Uses SampledFrom to mix empty strings, unicode, and typical values
		login := rapid.OneOf(
			rapid.String(),
			rapid.Just(""),
			rapid.StringMatching(`[a-z0-9._\-]{1,30}`),
		).Draw(t, "login")

		name := rapid.OneOf(
			rapid.String(),
			rapid.Just(""),
			rapid.StringMatching(`[A-Za-z ]{0,50}`),
		).Draw(t, "name")

		email := rapid.OneOf(
			rapid.String(),
			rapid.Just(""),
			rapid.StringMatching(`[a-z]+@[a-z]+\.[a-z]{2,4}`),
		).Draw(t, "email")

		banned := rapid.Bool().Draw(t, "banned")

		yt := model.YTUser{
			Login:  login,
			Name:   name,
			Email:  email,
			Banned: banned,
		}

		result := ConvertYTUserToJira(yt)

		// Preservation assertions: the mapping logic must remain unchanged
		if result.Key != yt.Login {
			t.Fatalf("Key = %q, want %q (login)", result.Key, yt.Login)
		}
		if result.Name != yt.Login {
			t.Fatalf("Name = %q, want %q (login)", result.Name, yt.Login)
		}
		if result.DisplayName != yt.Name {
			t.Fatalf("DisplayName = %q, want %q (Name)", result.DisplayName, yt.Name)
		}
		if result.EmailAddress != yt.Email {
			t.Fatalf("EmailAddress = %q, want %q (Email)", result.EmailAddress, yt.Email)
		}
		if result.Active != !yt.Banned {
			t.Fatalf("Active = %v, want %v (!Banned)", result.Active, !yt.Banned)
		}
	})
}

// TestPropertyPreservationConvertYTUsersToJira validates that ConvertYTUsersToJira
// preserves order and count: the output slice has the same length as input, and each
// element at index i corresponds to the conversion of input[i].
//
// **Validates: Requirements 3.1, 3.3**
func TestPropertyPreservationConvertYTUsersToJira(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		// Generate a slice of random YTUser structs (0 to 20 elements)
		count := rapid.IntRange(0, 20).Draw(t, "count")
		users := make([]model.YTUser, count)
		for i := range users {
			users[i] = model.YTUser{
				Login:  rapid.String().Draw(t, "login"),
				Name:   rapid.String().Draw(t, "name"),
				Email:  rapid.String().Draw(t, "email"),
				Banned: rapid.Bool().Draw(t, "banned"),
			}
		}

		results := ConvertYTUsersToJira(users)

		// Assert count is preserved
		if len(results) != len(users) {
			t.Fatalf("result length = %d, want %d", len(results), len(users))
		}

		// Assert order is preserved: each result[i] matches the expected conversion of users[i]
		for i, yt := range users {
			r := results[i]
			if r.Key != yt.Login {
				t.Fatalf("results[%d].Key = %q, want %q", i, r.Key, yt.Login)
			}
			if r.Name != yt.Login {
				t.Fatalf("results[%d].Name = %q, want %q", i, r.Name, yt.Login)
			}
			if r.DisplayName != yt.Name {
				t.Fatalf("results[%d].DisplayName = %q, want %q", i, r.DisplayName, yt.Name)
			}
			if r.EmailAddress != yt.Email {
				t.Fatalf("results[%d].EmailAddress = %q, want %q", i, r.EmailAddress, yt.Email)
			}
			if r.Active != !yt.Banned {
				t.Fatalf("results[%d].Active = %v, want %v", i, r.Active, !yt.Banned)
			}
		}
	})
}
