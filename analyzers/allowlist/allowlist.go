// Package allowlist provides a shared parser for PlatformKit's time-expiring
// allowlist files. All analyzers in the pkvet suite use this package.
//
// Format:
//
//	rule:subject|owner=@team|until=2026-06-30|reason=explanation text
//
// Entries past their until date are rejected at load time.
package allowlist

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// Entry represents a single parsed allowlist entry.
type Entry struct {
	Rule    string
	Subject string
	Owner   string
	Until   time.Time
	Reason  string
}

// Entries is a collection of allowlist entries with lookup methods.
type Entries []Entry

// Load reads an allowlist file and returns parsed entries.
// It returns an error for malformed entries or entries past their expiry date.
// Empty lines and lines starting with # are skipped.
func Load(path string) (Entries, error) {
	if path == "" {
		return nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() {
		// justified: read-only file; the close error is non-actionable.
		_ = f.Close()
	}()

	var entries Entries
	scanner := bufio.NewScanner(f)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		entry, err := parseLine(line)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNum, err)
		}

		if time.Now().After(entry.Until) {
			return nil, fmt.Errorf("%s:%d: expired allowlist entry (until=%s): %s",
				path, lineNum, entry.Until.Format("2006-01-02"), line)
		}

		entries = append(entries, entry)
	}

	return entries, scanner.Err()
}

// IsAllowed checks if a rule:subject pair is covered by any entry.
func (e Entries) IsAllowed(rule, subject string) bool {
	for _, entry := range e {
		if entry.Rule == rule && entry.Subject == subject {
			return true
		}
	}
	return false
}

// IsSubjectAllowed checks if a subject is covered under any rule.
func (e Entries) IsSubjectAllowed(subject string) bool {
	for _, entry := range e {
		if entry.Subject == subject {
			return true
		}
	}
	return false
}

func parseLine(line string) (Entry, error) {
	parts := strings.SplitN(line, "|", 4)
	if len(parts) < 3 {
		return Entry{}, fmt.Errorf("expected at least rule:subject|owner=...|until=YYYY-MM-DD, got: %s", line)
	}

	// Parse rule:subject
	ruleSubject := strings.TrimSpace(parts[0])
	before, after, ok := strings.Cut(ruleSubject, ":")
	if !ok {
		return Entry{}, fmt.Errorf("expected rule:subject, got: %s", ruleSubject)
	}

	entry := Entry{
		Rule:    before,
		Subject: after,
	}

	// Parse remaining key=value fields
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(part, "owner="):
			entry.Owner = strings.TrimPrefix(part, "owner=")
		case strings.HasPrefix(part, "until="):
			dateStr := strings.TrimPrefix(part, "until=")
			t, err := time.Parse("2006-01-02", dateStr)
			if err != nil {
				return Entry{}, fmt.Errorf("invalid until date %q: %w", dateStr, err)
			}
			// Set to end of day so the entry is valid through the entire until date.
			entry.Until = t.Add(24*time.Hour - time.Second)
		case strings.HasPrefix(part, "reason="):
			entry.Reason = strings.TrimPrefix(part, "reason=")
		default:
			return Entry{}, fmt.Errorf("unknown field %q", part)
		}
	}

	if entry.Owner == "" {
		return Entry{}, fmt.Errorf("missing owner= field")
	}
	if entry.Until.IsZero() {
		return Entry{}, fmt.Errorf("missing until= field")
	}

	return entry, nil
}
