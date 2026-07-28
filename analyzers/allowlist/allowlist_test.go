package allowlist

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadValidEntries(t *testing.T) {
	t.Parallel()
	content := `# comment
missing_access_contract:chat_management|owner=@platform|until=2099-12-31|reason=access not yet wired
boundary:auth_management|owner=@security|until=2099-06-30
`
	path := writeTemp(t, content)
	entries, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Rule != "missing_access_contract" || entries[0].Subject != "chat_management" {
		t.Fatalf("unexpected entry 0: %+v", entries[0])
	}
	if entries[0].Owner != "@platform" {
		t.Fatalf("unexpected owner: %s", entries[0].Owner)
	}
	if entries[0].Reason != "access not yet wired" {
		t.Fatalf("unexpected reason: %s", entries[0].Reason)
	}
}

func TestLoadRejectsExpiredEntry(t *testing.T) {
	t.Parallel()
	content := `expired_rule:some_module|owner=@team|until=2020-01-01`
	path := writeTemp(t, content)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for expired entry")
	}
}

func TestLoadRejectsMalformed(t *testing.T) {
	t.Parallel()
	cases := []string{
		"no-colon-here|owner=@team|until=2099-01-01",
		"rule:subject|until=2099-01-01",
		"rule:subject|owner=@team",
		"rule:subject|owner=@team|until=not-a-date",
	}
	for _, c := range cases {
		path := writeTemp(t, c)
		_, err := Load(path)
		if err == nil {
			t.Fatalf("expected error for: %s", c)
		}
	}
}

func TestIsAllowed(t *testing.T) {
	t.Parallel()
	entries := Entries{
		{Rule: "boundary", Subject: "auth_management", Owner: "@team", Until: time.Now().Add(time.Hour)},
	}
	if !entries.IsAllowed("boundary", "auth_management") {
		t.Fatal("expected allowed")
	}
	if entries.IsAllowed("boundary", "user_management") {
		t.Fatal("expected not allowed")
	}
	if entries.IsAllowed("other_rule", "auth_management") {
		t.Fatal("expected not allowed for different rule")
	}
}

func TestLoadMissingFileReturnsNil(t *testing.T) {
	t.Parallel()
	entries, err := Load("/nonexistent/path/file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries != nil {
		t.Fatal("expected nil entries for missing file")
	}
}

func TestLoadEmptyPathReturnsNil(t *testing.T) {
	t.Parallel()
	entries, err := Load("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries != nil {
		t.Fatal("expected nil entries for empty path")
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "allowlist.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
