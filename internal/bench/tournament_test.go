package bench

import (
	"testing"
)

func TestResearchTournament(t *testing.T) {
	// 1. Test Elo update math
	rA, rB := 1500.0, 1500.0
	// If A wins, rating must increase
	newA, newB := ComputeEloUpdate(rA, rB, 1.0)
	if newA <= rA || newB >= rB {
		t.Fatalf("expected A to gain rating and B to lose, got newA=%.1f, newB=%.1f", newA, newB)
	}

	// 2. Run Tournament
	report := RunResearchTournament()
	if report.Status != "GREEN" {
		t.Fatalf("expected tournament status GREEN, got %s", report.Status)
	}

	if report.TotalMatches < 50 {
		t.Fatalf("expected at least 50 matches in round-robin, got %d", report.TotalMatches)
	}

	if len(report.Standings) < 10 {
		t.Fatalf("expected at least 10 competitors in tournament, got %d", len(report.Standings))
	}

	// Winner must be a top-performing candidate
	if report.Winner.EloRating < 1800 {
		t.Fatalf("expected tournament winner to have Elo >= 1800, got %.1f", report.Winner.EloRating)
	}

	// Table format string must not be empty
	table := FormatTournamentTable(report)
	if len(table) == 0 {
		t.Fatalf("expected non-empty formatted tournament table")
	}
}
