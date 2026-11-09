package main

// Routing: a given path selects the correct installation id; unknown owner → error.
//
// White-box unit tests for resolveInstallation, which extracts the repo owner from the
// git credential `path` field (first "/" segment) and looks it up in the installations map.

import (
	"testing"
)

// TestResolveInstallation_KnownOwner verifies that a path for a known owner returns
// the correct installation ID without error.
func TestResolveInstallation_KnownOwner(t *testing.T) {
	// Given: a config with two known owners
	cfg := &Config{
		Installations: map[string]int64{
			"myorg": 12345,
			"peter": 67890,
		},
	}

	// When: resolveInstallation is called with "myorg/some-repo"
	id, err := resolveInstallation(cfg, "myorg/some-repo")

	// Then: the correct installation ID is returned
	if err != nil {
		t.Fatalf("unexpected error for known owner: %v", err)
	}
	if id != 12345 {
		t.Errorf("installation ID: got %d, want 12345", id)
	}
}

// TestResolveInstallation_SecondKnownOwner verifies routing to the second entry in
// a multi-owner config is not masked by the first.
func TestResolveInstallation_SecondKnownOwner(t *testing.T) {
	// Given: a config with two known owners
	cfg := &Config{
		Installations: map[string]int64{
			"myorg": 12345,
			"peter": 67890,
		},
	}

	// When: resolveInstallation is called with "peter/repo"
	id, err := resolveInstallation(cfg, "peter/repo")

	// Then: the second owner's ID is returned
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 67890 {
		t.Errorf("installation ID: got %d, want 67890", id)
	}
}

// TestResolveInstallation_UnknownOwnerFailsClosed verifies that an unrecognized owner
// returns an error, enforcing the "fail closed" security invariant.
func TestResolveInstallation_UnknownOwnerFailsClosed(t *testing.T) {
	// Given: a config with known owners
	cfg := &Config{
		Installations: map[string]int64{
			"myorg": 12345,
		},
	}

	// When: an unknown owner path is given
	_, err := resolveInstallation(cfg, "unknown-org/repo")

	// Then: an error is returned — fail closed, no installation ID guessed
	if err == nil {
		t.Fatal("expected error for unknown owner (fail-closed), got nil")
	}
}

// TestResolveInstallation_ExtractsFirstSegment verifies that only the first "/"
// segment is used as the owner key, ignoring sub-paths.
func TestResolveInstallation_ExtractsFirstSegment(t *testing.T) {
	// Given: a config with a known owner
	cfg := &Config{
		Installations: map[string]int64{
			"acme": 999,
		},
	}

	// When: a multi-segment path is given (e.g. owner/org/repo)
	id, err := resolveInstallation(cfg, "acme/widgets/extra")

	// Then: first segment "acme" is used; ID is returned
	if err != nil {
		t.Fatalf("unexpected error for multi-segment path: %v", err)
	}
	if id != 999 {
		t.Errorf("installation ID: got %d, want 999", id)
	}
}

// TestResolveInstallation_OwnerOnlyPath verifies that a bare owner string with no
// slash is also routed correctly.
func TestResolveInstallation_OwnerOnlyPath(t *testing.T) {
	// Given: a config with a known owner
	cfg := &Config{
		Installations: map[string]int64{
			"myorg": 42,
		},
	}

	// When: only the owner name (no slash) is given
	id, err := resolveInstallation(cfg, "myorg")

	// Then: the owner is matched and its ID is returned
	if err != nil {
		t.Fatalf("unexpected error for owner-only path: %v", err)
	}
	if id != 42 {
		t.Errorf("installation ID: got %d, want 42", id)
	}
}

// TestResolveInstallation_EmptyPathFailsClosed verifies that an empty path returns
// an error rather than selecting a default installation.
func TestResolveInstallation_EmptyPathFailsClosed(t *testing.T) {
	// Given: a config with installations
	cfg := &Config{
		Installations: map[string]int64{"myorg": 1},
	}

	// When: an empty path is given
	_, err := resolveInstallation(cfg, "")

	// Then: an error is returned — fail closed
	if err == nil {
		t.Fatal("expected error for empty path (fail-closed), got nil")
	}
}

// TestResolveInstallation_LeadingSlashEmptyOwner verifies that a path beginning
// with "/" (e.g. "/owner/repo") extracts an empty owner string and fails closed.
//
// When repoPath="/owner/repo", strings.IndexByte finds "/" at index 0, so
// owner = repoPath[:0] = "". The empty string is not in any installations
// map → error. This test pins the fail-closed behaviour for this edge case.
func TestResolveInstallation_LeadingSlashEmptyOwner(t *testing.T) {
	// Given: a config that knows about "owner" by its bare name (no leading slash).
	cfg := &Config{
		Installations: map[string]int64{
			"owner": 42,
		},
	}

	// When: a leading-slash path is given (first "/" segment produces empty string).
	_, err := resolveInstallation(cfg, "/owner/repo")

	// Then: an error is returned (empty owner is not in the map — fail closed).
	if err == nil {
		t.Fatal("expected error for leading-slash path '/owner/repo' (owner segment is empty), got nil")
	}
}

// TestResolveInstallation_TableDriven exercises path parsing boundary cases.
func TestResolveInstallation_TableDriven(t *testing.T) {
	cfg := &Config{
		Installations: map[string]int64{
			"alpha": 1,
			"beta":  2,
		},
	}

	tests := []struct {
		name    string
		path    string
		wantID  int64
		wantErr bool
	}{
		{name: "simple owner/repo", path: "alpha/repo", wantID: 1, wantErr: false},
		{name: "owner only", path: "beta", wantID: 2, wantErr: false},
		{name: "deep path", path: "alpha/a/b/c", wantID: 1, wantErr: false},
		{name: "unknown owner", path: "gamma/repo", wantErr: true},
		{name: "empty path", path: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Given: the table case
			// When: resolveInstallation is called
			id, err := resolveInstallation(cfg, tc.path)

			// Then: error state matches expectation
			if tc.wantErr {
				if err == nil {
					t.Errorf("path %q: expected error, got nil (id=%d)", tc.path, id)
				}
			} else {
				if err != nil {
					t.Errorf("path %q: unexpected error: %v", tc.path, err)
				}
				if id != tc.wantID {
					t.Errorf("path %q: got id %d, want %d", tc.path, id, tc.wantID)
				}
			}
		})
	}
}
