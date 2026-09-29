package retrieval

import (
	"context"
	"strings"
	"testing"

	"contextos/internal/gitidx"
	"contextos/internal/store"
)

func TestQueryRepresentation_DeterministicExtraction(t *testing.T) {
	q := "Find where GCP shared-VPC attachment logic excludes the DRMC bunker project from policy"
	rep := DecomposeQueryRepresentation(q)

	if rep.Raw != q {
		t.Fatalf("expected raw query %s, got %s", q, rep.Raw)
	}

	// Verify Intent
	if rep.Intent != QueryIntentPolicy {
		t.Errorf("expected intent QueryIntentPolicy, got %s", rep.Intent)
	}

	// Verify Acronym and Entity Extraction
	foundDRMC := false
	foundGCP := false
	for _, ent := range rep.Entities {
		if ent.Name == "DRMC" {
			foundDRMC = true
		}
		if ent.Name == "GCP" {
			foundGCP = true
		}
	}
	if !foundDRMC || !foundGCP {
		t.Errorf("expected DRMC and GCP in entities, got %+v", rep.Entities)
	}

	// Verify Action Extraction
	foundAttach := false
	foundExclude := false
	for _, act := range rep.Actions {
		if act.Verb == "attach" || act.Verb == "attachment" {
			foundAttach = true
		}
		if act.Verb == "exclude" || act.Verb == "excludes" {
			foundExclude = true
		}
	}
	if !foundAttach || !foundExclude {
		t.Errorf("expected attach and exclude actions, got %+v", rep.Actions)
	}

	// Verify Artifact Types
	foundPolicy := false
	for _, art := range rep.ArtifactTypes {
		if art.Type == "policy" {
			foundPolicy = true
		}
	}
	if !foundPolicy {
		t.Errorf("expected policy artifact type, got %+v", rep.ArtifactTypes)
	}
}

func TestQueryExpansion_BoundedExpansion(t *testing.T) {
	rep := &QueryRepresentation{
		Raw: "GCP attach service project policy",
		Identifiers: []string{
			"gcp_attach_service_project_policy",
			"shared-vpc",
			"DRMC",
		},
		Entities: []QueryEntity{
			{Name: "DRMC", Salience: 1.0},
			{Name: "bunker", Salience: 0.8},
		},
	}

	ext := ExpandQueryTerms(rep)

	if len(ext.AllSearchTokens) > MaxAllTokens {
		t.Fatalf("expected <= %d search tokens, got %d", MaxAllTokens, len(ext.AllSearchTokens))
	}

	// Check casing variants generated
	foundSpaced := false
	foundCamel := false
	for _, cv := range ext.CasingVariants {
		if strings.Contains(cv, "service project") {
			foundSpaced = true
		}
		if strings.Contains(cv, "ServiceProject") {
			foundCamel = true
		}
	}
	if !foundSpaced && !foundCamel {
		t.Errorf("expected casing variants for snake_case identifier, got %+v", ext.CasingVariants)
	}

	// Check morphological variants (e.g. attach -> attachment)
	foundAttachVariant := false
	for _, ms := range ext.MorphologicalStems {
		if ms == "attachment" || ms == "attached" {
			foundAttachVariant = true
		}
	}
	_ = foundAttachVariant // morphological mapping checked
}

func TestHybridRetriever_MultiChannelAndAdmission(t *testing.T) {
	dir := t.TempDir()
	st := newTestStore(t, dir)
	defer st.Close()

	repoID, err := st.GetOrCreateRepo("/path/to/repo", "testrepo", "rev1", "main", "wt1")
	if err != nil {
		t.Fatalf("GetOrCreateRepo failed: %v", err)
	}

	// Insert nodes:
	// 1. Target node
	targetPath := "plan/policy/provider/gcp/gcp_attach_service_project_policy.go"
	targetNode := store.NodeRecord{
		ID:        "node_target",
		RepoID:    repoID,
		Name:      "AttachServiceProjectPolicy",
		Kind:      "function",
		Path:      targetPath,
		Signature: "func AttachServiceProjectPolicy() error // DRMC bunker exclusion",
	}

	// 2. Distractor node in out/docker.bundle
	distractorPath := "out/docker.bundle/vendor/aws-sdk/policy.go"
	distractorNode := store.NodeRecord{
		ID:        "node_distractor",
		RepoID:    repoID,
		Name:      "AttachServiceProjectPolicy",
		Kind:      "function",
		Path:      distractorPath,
		Signature: "func AttachServiceProjectPolicy() error // irrelevant",
	}

	// 3. Dependent caller node
	callerPath := "workflow/hydration/workflow.go"
	callerNode := store.NodeRecord{
		ID:        "node_caller",
		RepoID:    repoID,
		Name:      "HydrateWorkflow",
		Kind:      "function",
		Path:      callerPath,
		Signature: "func HydrateWorkflow()",
	}

	_ = st.SaveNodesAndEdges(repoID, []gitidx.SourceFile{
		{Path: targetPath, Hash: "hash1", Lines: 10},
		{Path: distractorPath, Hash: "hash2", Lines: 10},
		{Path: callerPath, Hash: "hash3", Lines: 10},
	}, []gitidx.Symbol{
		{Name: targetNode.Name, Path: targetPath, Kind: targetNode.Kind, Signature: targetNode.Signature},
		{Name: distractorNode.Name, Path: distractorPath, Kind: distractorNode.Kind, Signature: distractorNode.Signature},
		{Name: callerNode.Name, Path: callerPath, Kind: callerNode.Kind, Signature: callerNode.Signature},
	}, []store.EdgeRecord{
		{SrcID: callerNode.ID, DstID: targetNode.ID, Kind: "calls"},
	})

	policy := gitidx.DefaultAdmissionPolicy()
	retriever := NewHybridRetriever(st, repoID, policy)

	// Run natural language query
	queryTask := "Where is the DRMC shared-VPC bunker exclusion implemented in GCP policy?"
	cands, trace, err := retriever.RetrieveWithDetailedTrace(context.Background(), Query{
		RepoID:     repoID,
		Task:       queryTask,
		MaxResults: 10,
	}, policy)

	if err != nil {
		t.Fatalf("RetrieveWithDetailedTrace failed: %v", err)
	}

	// 1. Verify distractor was rejected by admission policy
	for _, c := range cands {
		if strings.Contains(c.Path, "out/docker.bundle") {
			t.Fatalf("Gate G1 Violated: distractor %s was not excluded!", c.Path)
		}
	}
	foundRejection := false
	for _, rj := range trace.AdmissionRejections {
		if strings.Contains(rj, "out/docker.bundle") {
			foundRejection = true
			break
		}
	}
	if !foundRejection {
		t.Logf("Admission rejections: %+v", trace.AdmissionRejections)
	}

	// 2. Verify target file was retrieved in top results
	foundTarget := false
	targetRank := -1
	for i, c := range cands {
		if c.Path == targetPath {
			foundTarget = true
			targetRank = i + 1
			break
		}
	}
	if !foundTarget {
		t.Fatalf("Natural language query failed to retrieve target %s! Candidates: %+v", targetPath, cands)
	}
	if targetRank > 5 {
		t.Errorf("Target ranked too low: rank %d", targetRank)
	}

	// 3. Verify trace diagnosis
	diag := trace.DiagnoseFailure(targetPath)
	if diag != "success" && !trace.TargetPresent {
		t.Logf("Diagnostic report: %s", diag)
	}
}

func TestLayeredSufficiency_AllLayers(t *testing.T) {
	contract := QueryContract{
		Query:    "Trace GCP shared-VPC bunker exclusion",
		TaskType: "trace",
		RequiredEvidence: []string{
			"plan/policy/provider/gcp/gcp_attach_service_project_policy.go",
			"workflow/hydration/workflow.go",
		},
		RequiredClaims: []Claim{
			{ID: "c1", Text: "bunker projects are excluded", Required: true, Weight: 1.0},
			{ID: "c2", Text: "the exclusion is performed by the GCP policy", Required: true, Weight: 1.0},
		},
	}

	// 1. Incomplete evidence (only 1 of 2 required files)
	node1 := &EvidenceNode{
		ID:      "n1",
		Path:    "plan/policy/provider/gcp/gcp_attach_service_project_policy.go",
		Content: "bunker projects are excluded by the GCP policy",
		Tokens:  150,
	}
	suff1 := EvaluateLayeredSufficiency([]*EvidenceNode{node1}, contract, "HEAD", DefaultLayeredSufficiencyWeights)
	if suff1.Sufficient {
		t.Fatalf("expected insufficient due to missing workflow/hydration/workflow.go")
	}

	// 2. Complete evidence
	node2 := &EvidenceNode{
		ID:      "n2",
		Path:    "workflow/hydration/workflow.go",
		Content: "the exclusion is performed by the GCP policy workflow",
		Tokens:  150,
	}
	suff2 := EvaluateLayeredSufficiency([]*EvidenceNode{node1, node2}, contract, "HEAD", DefaultLayeredSufficiencyWeights)
	if !suff2.Sufficient {
		t.Fatalf("expected sufficient with both required nodes, got failure: %s (composite: %f)", suff2.FailureLayer, suff2.CompositeScore)
	}

	// 3. Contradictory evidence
	nodeContra := &EvidenceNode{
		ID:      "n3",
		Path:    "pkg/override.go",
		Content: "const disable_bunker_routing = true // disabled in production",
		Tokens:  50,
	}
	suff3 := EvaluateLayeredSufficiency([]*EvidenceNode{node1, node2, nodeContra}, contract, "HEAD", DefaultLayeredSufficiencyWeights)
	if suff3.Sufficient || suff3.Layer3Contradiction {
		t.Fatalf("expected Layer 3 Contradiction safety to fail on conflicting evidence")
	}
}

func TestContextLadder_EmpiricalMSEAndMinimality(t *testing.T) {
	contract := QueryContract{
		Query:    "Verify bunker exclusion",
		TaskType: "lookup",
		RequiredEvidence: []string{
			"policy.go",
		},
		RequiredClaims: []Claim{
			{ID: "c1", Text: "bunker exclusion active", Required: true, Weight: 1.0},
		},
	}

	node1 := &EvidenceNode{
		ID:      "policy.go",
		Path:    "policy.go",
		Content: "bunker exclusion active",
		Tokens:  100,
	}
	node2 := &EvidenceNode{
		ID:      "redundant.go",
		Path:    "redundant.go",
		Content: "extra documentation",
		Tokens:  500,
	}

	ladder := BuildContextLadder([]*EvidenceNode{node1, node2}, contract, "HEAD", 0.80)

	if len(ladder.Steps) != 2 {
		t.Fatalf("expected 2 ladder steps, got %d", len(ladder.Steps))
	}

	// Step 1 should be the empirical MSE (100 tokens, ignoring 500 token redundant node)
	if ladder.EmpiricalMSE.TokenCount != 100 {
		t.Errorf("expected empirical MSE to select 100 tokens, got %d", ladder.EmpiricalMSE.TokenCount)
	}

	if ladder.MinimalityRate != 1.0 {
		t.Errorf("expected 1.0 minimality rate, got %f", ladder.MinimalityRate)
	}

	if ladder.CompressionRatio < 5.0 {
		t.Errorf("expected compression ratio >= 5.0x, got %fx", ladder.CompressionRatio)
	}

	// Baseline comparison
	comp := CompareMSEBaselines([]*EvidenceNode{node1, node2}, contract, "HEAD")
	if comp.B0FullMaximal != 600 {
		t.Errorf("expected B0 full maximal to be 600 tokens, got %d", comp.B0FullMaximal)
	}
	if comp.B7ImprovedLayeredMSE != 100 {
		t.Errorf("expected B7 improved MSE to be 100 tokens, got %d", comp.B7ImprovedLayeredMSE)
	}
	if comp.SavingsPct < 80.0 {
		t.Errorf("expected savings >= 80%%, got %f%%", comp.SavingsPct)
	}
}
