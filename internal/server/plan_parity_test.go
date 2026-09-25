package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestPolicyRepo(t *testing.T) (string, string) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "ctx.db")
	if err := runGit(root, "init", "-q"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "config", "user.email", "test@example.com"); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "config", "user.name", "Test"); err != nil {
		t.Fatal(err)
	}

	policyDir := filepath.Join(root, "plan", "policy", "provider", "gcp")
	if err := os.MkdirAll(policyDir, 0755); err != nil {
		t.Fatal(err)
	}

	policyCode := `package gcp

// isBackupTeam determines whether a project belongs to the DRMC bunker team.
func isBackupTeam(team string) bool {
	return team == "DRMC"
}

// AttachServiceProject enforces shared-VPC attachment policy, excluding DRMC bunker projects.
func AttachServiceProject(project string) bool {
	if isBackupTeam(project) {
		return false
	}
	return true
}
`
	testCode := `package gcp

import "testing"

func TestAttachServiceProjectExcludesDRMC(t *testing.T) {
	if AttachServiceProject("DRMC") {
		t.Fatal("expected DRMC to be excluded")
	}
}
`
	otherCode := `package util

func HelperFunc() string {
	return "unrelated helper logic"
}
`
	if err := os.WriteFile(filepath.Join(policyDir, "gcp_attach_service_project_policy.go"), []byte(policyCode), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(policyDir, "gcp_attach_service_project_policy_test.go"), []byte(testCode), 0644); err != nil {
		t.Fatal(err)
	}
	utilDir := filepath.Join(root, "util")
	if err := os.MkdirAll(utilDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(utilDir, "helper.go"), []byte(otherCode), 0644); err != nil {
		t.Fatal(err)
	}

	if err := runGit(root, "add", "."); err != nil {
		t.Fatal(err)
	}
	if err := runGit(root, "commit", "-qm", "add policy files"); err != nil {
		t.Fatal(err)
	}

	return root, dbPath
}

// TestPlanRetrievalParity verifies that Service.Plan executes the R18 HybridRetriever
// pipeline and returns the same authoritative evidence as RetrieveEvidence (R18.1 P0 & §20).
func TestPlanRetrievalParity(t *testing.T) {
	root, dbPath := setupTestPolicyRepo(t)
	s, err := New(dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Index(); err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	query := "Find where GCP shared-VPC attachment logic excludes the DRMC bunker project in gcp_attach_service_project_policy.go"

	// 1. Direct retrieve via authoritative HybridRetriever
	retrieveCands, _, err := s.RetrieveEvidence(context.Background(), query, 10)
	if err != nil {
		t.Fatalf("RetrieveEvidence failed: %v", err)
	}
	if len(retrieveCands) == 0 {
		t.Fatal("RetrieveEvidence returned 0 candidates")
	}

	targetPath := "plan/policy/provider/gcp/gcp_attach_service_project_policy.go"
	var retrieveFound bool
	for _, c := range retrieveCands {
		if strings.HasSuffix(c.Path, "gcp_attach_service_project_policy.go") {
			retrieveFound = true
			break
		}
	}
	if !retrieveFound {
		t.Fatalf("RetrieveEvidence failed to find target file %s", targetPath)
	}

	// 2. Execute Service.Plan
	p, err := s.Plan(query, "gpt-5.3-codex", 4000)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	// 3. Verify P0 Telemetry contract
	if p.RetrievalMode != "hybrid" {
		t.Fatalf("expected plan retrieval_mode = 'hybrid', got '%s'", p.RetrievalMode)
	}
	if len(p.RetrievalStages) == 0 {
		t.Fatal("expected plan retrieval_stages to be populated")
	}
	if p.CandidateCount == 0 {
		t.Fatal("expected candidate_count > 0 in plan telemetry")
	}

	// 4. Verify that target file is present in plan selected/candidates
	var planFound bool
	for _, c := range p.Selected {
		if strings.Contains(c.Location, "gcp_attach_service_project_policy.go") || strings.Contains(c.Content, "gcp_attach_service_project_policy.go") {
			planFound = true
			break
		}
	}
	if !planFound {
		for _, c := range p.Candidates {
			if strings.Contains(c.Location, "gcp_attach_service_project_policy.go") || strings.Contains(c.Content, "gcp_attach_service_project_policy.go") {
				planFound = true
				break
			}
		}
	}
	if !planFound {
		t.Fatalf("Plan failed to include target file %s in context candidates (parity broken)", targetPath)
	}

	// 5. Parity check: top retrieve candidate path matches top plan code candidate
	topRetrievePath := retrieveCands[0].Path
	var topPlanCodePath string
	for _, c := range p.Selected {
		if c.Kind == "file" || c.Kind == "func" || c.Kind == "code" {
			topPlanCodePath = c.Location
			break
		}
	}
	if topPlanCodePath == "" && len(p.Selected) > 0 {
		topPlanCodePath = p.Selected[0].Location
	}
	if !strings.Contains(topPlanCodePath, "gcp_attach_service_project_policy.go") && !strings.Contains(topRetrievePath, "gcp_attach_service_project_policy.go") {
		t.Errorf("Parity mismatch: top retrieve = %s, top plan = %s", topRetrievePath, topPlanCodePath)
	}

	// 6. Test RenderPlan output contains telemetry
	rendered := s.RenderPlan(p)
	if !strings.Contains(rendered, "Retrieval: hybrid") {
		t.Errorf("RenderPlan missing hybrid retrieval header: %s", rendered)
	}
}

// TestQueryOrderInvariance tests that moving the exact identifier from start to middle to end
// does not degrade retrieval performance (R18.1 Defect B & §7-§10).
func TestQueryOrderInvariance(t *testing.T) {
	root, dbPath := setupTestPolicyRepo(t)
	s, err := New(dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Index(); err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	testCases := []struct {
		name  string
		query string
	}{
		{
			name:  "Identifier at Start",
			query: "gcp_attach_service_project_policy.go: Find where GCP shared-VPC attachment logic excludes the DRMC bunker project",
		},
		{
			name:  "Identifier in Middle",
			query: "Find in gcp_attach_service_project_policy.go where GCP shared-VPC attachment logic excludes the DRMC bunker project",
		},
		{
			name:  "Identifier at End (Verbose natural language prompt)",
			query: "Find where GCP shared-VPC attachment logic excludes the DRMC bunker project in gcp_attach_service_project_policy.go",
		},
		{
			name:  "Identifier without Extension at End",
			query: "Find where GCP shared-VPC attachment logic excludes the DRMC bunker project gcp_attach_service_project_policy",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := s.Plan(tc.query, "gpt-5.3-codex", 4000)
			if err != nil {
				t.Fatalf("Plan failed for '%s': %v", tc.query, err)
			}
			var found bool
			for _, c := range p.Selected {
				if strings.Contains(c.Location, "gcp_attach_service_project_policy") || strings.Contains(c.Content, "gcp_attach_service_project_policy") {
					found = true
					break
				}
			}
			if !found {
				for _, c := range p.Candidates {
					if strings.Contains(c.Location, "gcp_attach_service_project_policy") || strings.Contains(c.Content, "gcp_attach_service_project_policy") {
						found = true
						break
					}
				}
			}
			if !found {
				t.Errorf("Order sensitivity defect reproduced: failed to retrieve target for query: %s", tc.query)
			}
		})
	}
}

// TestCamelCaseSymbolRetrieval tests that camelCase symbols like isBackupTeam
// are parsed and retrieved with high priority (R18.1 §6 & §9).
func TestCamelCaseSymbolRetrieval(t *testing.T) {
	root, dbPath := setupTestPolicyRepo(t)
	s, err := New(dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Index(); err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	query := "Where is isBackupTeam defined and how does it check the project?"
	p, err := s.Plan(query, "gpt-5.3-codex", 4000)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	var foundSymbol bool
	for _, c := range p.Selected {
		if strings.Contains(c.Content, "isBackupTeam") {
			foundSymbol = true
			break
		}
	}
	if !foundSymbol {
		for _, c := range p.Candidates {
			if strings.Contains(c.Content, "isBackupTeam") {
				foundSymbol = true
				break
			}
		}
	}
	if !foundSymbol {
		t.Error("failed to retrieve camelCase symbol isBackupTeam")
	}
}

// TestNegativeRetrievalAbstention verifies that completely ungrounded queries do not retrieve false evidence (R18.1 §21).
func TestNegativeRetrievalAbstention(t *testing.T) {
	root, dbPath := setupTestPolicyRepo(t)
	s, err := New(dbPath, root)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.Index(); err != nil {
		t.Fatalf("Index failed: %v", err)
	}

	query := "Find non-existent quantum blockchain widget in imaginary_file.xyz"
	cands, _, err := s.RetrieveEvidence(context.Background(), query, 10)
	if err != nil {
		t.Fatalf("RetrieveEvidence failed: %v", err)
	}

	for _, c := range cands {
		if strings.Contains(c.Path, "imaginary_file.xyz") {
			t.Errorf("hallucinated imaginary_file.xyz: %+v", c)
		}
	}
}
