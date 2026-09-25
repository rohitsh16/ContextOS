package retrieval

// TaskRoutingProfile configures the channel fusion and reranking weights for a given QueryIntent (R18.1 §15).
type TaskRoutingProfile struct {
	Intent          QueryIntent         `json:"intent"`
	FusionWeights   MultiChannelWeights `json:"fusion_weights"`
	AuthorityWeight float64             `json:"authority_weight"`
	GraphDepth      int                 `json:"graph_depth"`
	MinScoreFloor   float64             `json:"min_score_floor"`
}

// GetRoutingProfile returns the optimal scoring weights conditioned on the query intent.
func GetRoutingProfile(intent QueryIntent) TaskRoutingProfile {
	switch intent {
	case QueryIntentTrace:
		return TaskRoutingProfile{
			Intent: intent,
			FusionWeights: MultiChannelWeights{
				Lexical:  0.10,
				Semantic: 0.15,
				Symbol:   0.25,
				Path:     0.15,
				Entity:   0.20,
				Graph:    0.15,
			},
			AuthorityWeight: 0.10,
			GraphDepth:      3, // deeper expansion for call chains
			MinScoreFloor:   0.10,
		}

	case QueryIntentPolicy:
		return TaskRoutingProfile{
			Intent: intent,
			FusionWeights: MultiChannelWeights{
				Lexical:  0.15,
				Semantic: 0.25,
				Symbol:   0.15,
				Path:     0.20,
				Entity:   0.20,
				Graph:    0.05,
			},
			AuthorityWeight: 0.15,
			GraphDepth:      2,
			MinScoreFloor:   0.15,
		}

	case QueryIntentDebug:
		return TaskRoutingProfile{
			Intent: intent,
			FusionWeights: MultiChannelWeights{
				Lexical:  0.20,
				Semantic: 0.25,
				Symbol:   0.20,
				Path:     0.10,
				Entity:   0.10,
				Graph:    0.15,
			},
			AuthorityWeight: 0.10,
			GraphDepth:      2,
			MinScoreFloor:   0.10,
		}

	case QueryIntentArchitecture:
		return TaskRoutingProfile{
			Intent: intent,
			FusionWeights: MultiChannelWeights{
				Lexical:  0.15,
				Semantic: 0.25,
				Symbol:   0.20,
				Path:     0.25,
				Entity:   0.10,
				Graph:    0.05,
			},
			AuthorityWeight: 0.15,
			GraphDepth:      2,
			MinScoreFloor:   0.10,
		}

	case QueryIntentTest:
		return TaskRoutingProfile{
			Intent: intent,
			FusionWeights: MultiChannelWeights{
				Lexical:  0.20,
				Semantic: 0.15,
				Symbol:   0.25,
				Path:     0.25,
				Entity:   0.10,
				Graph:    0.05,
			},
			AuthorityWeight: 0.10,
			GraphDepth:      1,
			MinScoreFloor:   0.10,
		}

	case QueryIntentComparison:
		return TaskRoutingProfile{
			Intent: intent,
			FusionWeights: MultiChannelWeights{
				Lexical:  0.25,
				Semantic: 0.30,
				Symbol:   0.15,
				Path:     0.15,
				Entity:   0.10,
				Graph:    0.05,
			},
			AuthorityWeight: 0.10,
			GraphDepth:      1,
			MinScoreFloor:   0.10,
		}

	default: // QueryIntentLookup
		return TaskRoutingProfile{
			Intent: intent,
			FusionWeights: MultiChannelWeights{
				Lexical:  0.20,
				Semantic: 0.20,
				Symbol:   0.25,
				Path:     0.25,
				Entity:   0.10,
				Graph:    0.00,
			},
			AuthorityWeight: 0.10,
			GraphDepth:      1,
			MinScoreFloor:   0.05,
		}
	}
}
