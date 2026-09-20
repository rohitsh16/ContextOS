package graph

import (
	"math"
	"testing"
)

func TestChainGraph(t *testing.T) {
	// A -> B -> C -> D
	g := New(DefaultConfig())
	for _, id := range []string{"A", "B", "C", "D"} {
		g.AddNode(&Node{ID: id, Name: id, Kind: "file", Path: id + ".go"})
	}
	g.AddEdge("A", "B", "call")
	g.AddEdge("B", "C", "call")
	g.AddEdge("C", "D", "call")

	// Seed at A
	seeds := map[string]float64{"A": 1.0}
	scores := g.ComputePPR(seeds)

	// Due to decay / restart, A > B > C > D
	if !(scores["A"] > scores["B"] && scores["B"] > scores["C"] && scores["C"] > scores["D"]) {
		t.Fatalf("expected A > B > C > D, got A=%f, B=%f, C=%f, D=%f",
			scores["A"], scores["B"], scores["C"], scores["D"])
	}
}

func TestDependencyTreeRanking(t *testing.T) {
	// task -> A -> B -> C
	//              \
	//               D
	g := New(DefaultConfig())
	for _, id := range []string{"A", "B", "C", "D"} {
		g.AddNode(&Node{ID: id, Name: id, Kind: "file", Path: id + ".go"})
	}
	g.AddEdge("A", "B", "import")
	g.AddEdge("B", "C", "call")
	g.AddEdge("B", "D", "call")

	seeds := map[string]float64{"A": 1.0}
	scores := g.ComputePPR(seeds)

	// A should rank highest, followed by B, then C and D
	if scores["A"] <= scores["B"] {
		t.Fatalf("expected A > B, got A=%f, B=%f", scores["A"], scores["B"])
	}
	if scores["B"] <= scores["C"] || scores["B"] <= scores["D"] {
		t.Fatalf("expected B > C and B > D, got B=%f, C=%f, D=%f", scores["B"], scores["C"], scores["D"])
	}
	// C and D are symmetric from B with equal edge weight
	if math.Abs(scores["C"]-scores["D"]) > 1e-4 {
		t.Fatalf("expected C ≈ D, got C=%f, D=%f", scores["C"], scores["D"])
	}
}

func TestDiamondGraph(t *testing.T) {
	// A -> B -> D
	// A -> C -> D
	g := New(DefaultConfig())
	for _, id := range []string{"A", "B", "C", "D"} {
		g.AddNode(&Node{ID: id, Name: id, Kind: "file", Path: id + ".go"})
	}
	g.AddEdge("A", "B", "call")
	g.AddEdge("A", "C", "call")
	g.AddEdge("B", "D", "call")
	g.AddEdge("C", "D", "call")

	seeds := map[string]float64{"A": 1.0}
	scores := g.ComputePPR(seeds)

	if scores["A"] <= scores["B"] {
		t.Fatalf("expected A > B, got A=%f, B=%f", scores["A"], scores["B"])
	}
	// B and C should have equal score
	if math.Abs(scores["B"]-scores["C"]) > 1e-4 {
		t.Fatalf("expected B ≈ C, got B=%f, C=%f", scores["B"], scores["C"])
	}
	// D receives transitions from both B and C
	if scores["D"] <= 0 {
		t.Fatalf("expected D > 0, got %f", scores["D"])
	}
}

func TestCyclicGraphConvergence(t *testing.T) {
	// A -> B -> C -> A (cycle)
	g := New(DefaultConfig())
	for _, id := range []string{"A", "B", "C"} {
		g.AddNode(&Node{ID: id, Name: id, Kind: "file", Path: id + ".go"})
	}
	g.AddEdge("A", "B", "call")
	g.AddEdge("B", "C", "call")
	g.AddEdge("C", "A", "call")

	seeds := map[string]float64{"A": 1.0}
	scores := g.ComputePPR(seeds)

	// In cycle with seed A, A has highest score due to alpha restart
	if scores["A"] < scores["B"] || scores["B"] < scores["C"] {
		t.Fatalf("expected A >= B >= C, got A=%f, B=%f, C=%f", scores["A"], scores["B"], scores["C"])
	}
}

func TestDisconnectedNode(t *testing.T) {
	g := New(DefaultConfig())
	g.AddNode(&Node{ID: "A", Name: "A", Kind: "file", Path: "a.go"})
	g.AddNode(&Node{ID: "B", Name: "B", Kind: "file", Path: "b.go"})
	g.AddNode(&Node{ID: "Isolated", Name: "Isolated", Kind: "file", Path: "iso.go"})

	g.AddEdge("A", "B", "call")

	seeds := map[string]float64{"A": 1.0}
	scores := g.ComputePPR(seeds)

	if scores["Isolated"] != 0 {
		t.Fatalf("expected 0 for isolated non-seed node, got %f", scores["Isolated"])
	}
	if scores["A"] <= scores["B"] {
		t.Fatalf("expected A > B, got A=%f, B=%f", scores["A"], scores["B"])
	}
}

func TestStarAndHubSpoke(t *testing.T) {
	// Hub connected to N spokes
	g := New(DefaultConfig())
	g.AddNode(&Node{ID: "Hub", Name: "Hub", Kind: "file", Path: "hub.go"})
	for i := 1; i <= 5; i++ {
		name := string(rune('0' + i))
		g.AddNode(&Node{ID: "Spoke" + name, Name: "Spoke" + name, Kind: "file", Path: "spoke" + name + ".go"})
		g.AddEdge("Hub", "Spoke"+name, "call")
	}

	seeds := map[string]float64{"Hub": 1.0}
	scores := g.ComputePPR(seeds)

	if scores["Hub"] != 1.0 {
		t.Fatalf("expected Hub to be max normalized score 1.0, got %f", scores["Hub"])
	}
	// All spokes should have identical score
	spoke1 := scores["Spoke1"]
	for i := 2; i <= 5; i++ {
		name := "Spoke" + string(rune('0'+i))
		if math.Abs(scores[name]-spoke1) > 1e-4 {
			t.Fatalf("expected %s ≈ Spoke1 (%f), got %f", name, spoke1, scores[name])
		}
	}
}

func TestDegreeNormalization(t *testing.T) {
	g := New(DefaultConfig())
	g.AddNode(&Node{ID: "Hub", Name: "Hub", Kind: "file", Path: "hub.go"})
	g.AddNode(&Node{ID: "Leaf", Name: "Leaf", Kind: "file", Path: "leaf.go"})
	g.AddNode(&Node{ID: "Isolated", Name: "Isolated", Kind: "file", Path: "iso.go"})

	g.AddEdge("Hub", "Leaf", "call")

	degHub := g.DegreeNorm("Hub")
	degLeaf := g.DegreeNorm("Leaf")
	degIso := g.DegreeNorm("Isolated")

	if degHub <= 0 || degHub > 1.0 {
		t.Fatalf("expected degHub in (0, 1], got %f", degHub)
	}
	if degLeaf <= 0 || degLeaf > 1.0 {
		t.Fatalf("expected degLeaf in (0, 1], got %f", degLeaf)
	}
	if degIso != 0 {
		t.Fatalf("expected degIso == 0, got %f", degIso)
	}
}

func TestTestCentrality(t *testing.T) {
	g := New(DefaultConfig())
	g.AddNode(&Node{ID: "service", Name: "service", Kind: "file", Path: "service.go"})
	g.AddNode(&Node{ID: "service_test", Name: "service_test", Kind: "test", Path: "service_test.go"})

	g.AddEdge("service_test", "service", "test-reference")

	tc := g.TestCentrality("service")
	if tc <= 0 {
		t.Fatalf("expected positive test centrality for node referenced by test, got %f", tc)
	}
	tcNone := g.TestCentrality("service_test")
	if tcNone != 0 {
		t.Fatalf("expected 0 test centrality for unreferenced test node, got %f", tcNone)
	}
}

func TestExtractSeedsFromQueryAndGit(t *testing.T) {
	g := New(DefaultConfig())
	g.AddNode(&Node{ID: "auth", Name: "AuthService", Kind: "file", Path: "internal/auth/auth.go"})
	g.AddNode(&Node{ID: "billing", Name: "BillingService", Kind: "file", Path: "internal/billing/billing.go"})

	// Query about auth
	seeds := g.ExtractSeeds("Fix token expiration in AuthService", []string{"internal/auth/auth.go"}, []string{"AuthService"})

	if seeds["auth"] <= 0 {
		t.Fatalf("expected auth node to be seeded, got %f", seeds["auth"])
	}
	if seeds["auth"] <= seeds["billing"] {
		t.Fatalf("expected auth seed weight > billing, got auth=%f, billing=%f", seeds["auth"], seeds["billing"])
	}
}
