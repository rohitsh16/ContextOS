package textutil

import (
	"testing"
)

func TestClassifyQueryTokens(t *testing.T) {
	q := "Find where GCP shared-VPC attachment logic excludes the DRMC bunker project gcp_attach_service_project_policy.go and check isBackupTeam"
	tokens := ClassifyQueryTokens(q)

	if len(tokens) == 0 {
		t.Fatal("expected classified tokens, got none")
	}

	// 1. Verify that gcp_attach_service_project_policy.go is ClassPathFilename and priority 1
	var foundFile, foundCamel, foundAcronym, foundGeneric bool
	for _, tok := range tokens {
		switch tok.Text {
		case "gcp_attach_service_project_policy.go":
			foundFile = true
			if tok.Class != ClassPathFilename || tok.Priority != 1 {
				t.Errorf("expected ClassPathFilename with prio 1, got %v (%d)", tok.Class, tok.Priority)
			}
			// Verify bounded variants
			var hasBase bool
			for _, v := range tok.Variants {
				if v == "gcp_attach_service_project_policy" {
					hasBase = true
				}
			}
			if !hasBase {
				t.Errorf("expected variant gcp_attach_service_project_policy in %v", tok.Variants)
			}
		case "isBackupTeam":
			foundCamel = true
			if tok.Class != ClassSymbol || tok.Priority != 2 {
				t.Errorf("expected ClassSymbol with prio 2, got %v (%d)", tok.Class, tok.Priority)
			}
			var hasSnake bool
			for _, v := range tok.Variants {
				if v == "is_backup_team" {
					hasSnake = true
				}
			}
			if !hasSnake {
				t.Errorf("expected variant is_backup_team in %v", tok.Variants)
			}
		case "GCP", "DRMC":
			foundAcronym = true
			if tok.Class != ClassEntity {
				t.Errorf("expected ClassEntity for %s, got %v", tok.Text, tok.Class)
			}
		case "find", "where", "logic":
			foundGeneric = true
			if tok.Class != ClassGeneric || tok.Priority != 7 {
				t.Errorf("expected ClassGeneric with prio 7 for %s, got %v (%d)", tok.Text, tok.Class, tok.Priority)
			}
		}
	}

	if !foundFile {
		t.Error("expected to find gcp_attach_service_project_policy.go")
	}
	if !foundCamel {
		t.Error("expected to find isBackupTeam")
	}
	if !foundAcronym {
		t.Error("expected to find acronym entities")
	}
	if !foundGeneric {
		t.Error("expected to find generic words downweighted")
	}

	// 2. Verify priority order: filenames and symbols MUST precede generic words
	firstPrio := tokens[0].Priority
	lastPrio := tokens[len(tokens)-1].Priority
	if firstPrio > 2 {
		t.Errorf("expected high priority token at start, got prio %d (%s)", firstPrio, tokens[0].Text)
	}
	if lastPrio < 5 {
		t.Errorf("expected low priority token at end, got prio %d (%s)", lastPrio, tokens[len(tokens)-1].Text)
	}

	// 3. Verify ExtractPrioritizedTokens
	prioritized := ExtractPrioritizedTokens(q)
	if len(prioritized) == 0 {
		t.Fatal("expected prioritized tokens, got none")
	}
	// The first token should be the filename or symbol, not "find" or "where"
	if prioritized[0] == "find" || prioritized[0] == "where" {
		t.Errorf("generic boilerplate leaked to top of prioritized tokens: %s", prioritized[0])
	}
}
