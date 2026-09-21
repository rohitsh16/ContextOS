package index

import (
	"sync"
)

// Shard represents a partition of the repository index by package or directory (PR.md Section 10).
type Shard struct {
	ID          string
	Package     string
	SegmentMgr  *SegmentManager
	DocToNodeID map[uint32]string
	NodeToDocID map[string]uint32
	mu          sync.RWMutex
}

// NewShard creates a new Shard for a package.
func NewShard(id, pkg string) *Shard {
	return &Shard{
		ID:          id,
		Package:     pkg,
		SegmentMgr:  NewSegmentManager(),
		DocToNodeID: make(map[uint32]string),
		NodeToDocID: make(map[string]uint32),
	}
}

// RegisterNode associates a node string ID with an internal docID in this shard.
func (s *Shard) RegisterNode(nodeID string) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()

	if docID, exists := s.NodeToDocID[nodeID]; exists {
		return docID
	}
	docID := s.SegmentMgr.NextDocID()
	s.NodeToDocID[nodeID] = docID
	s.DocToNodeID[docID] = nodeID
	return docID
}

// GetNodeID returns the string node ID for an internal docID.
func (s *Shard) GetNodeID(docID uint32) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.DocToNodeID[docID]
}

// ShardRouter routes queries to relevant shards and manages bounded concurrency (PR.md Section 10).
type ShardRouter struct {
	mu             sync.RWMutex
	shards         map[string]*Shard
	defaultShard   *Shard
	maxConcurrency int
}

// NewShardRouter creates a ShardRouter with bounded worker parallelism.
func NewShardRouter(maxConcurrency int) *ShardRouter {
	if maxConcurrency <= 0 {
		maxConcurrency = 4
	}
	return &ShardRouter{
		shards:         make(map[string]*Shard),
		defaultShard:   NewShard("shard_default", "default"),
		maxConcurrency: maxConcurrency,
	}
}

// GetOrCreateShard returns the shard for a package, creating it if necessary.
func (sr *ShardRouter) GetOrCreateShard(pkg string) *Shard {
	if pkg == "" {
		pkg = "default"
	}

	sr.mu.Lock()
	defer sr.mu.Unlock()

	if s, ok := sr.shards[pkg]; ok {
		return s
	}
	shardID := "shard_" + pkg
	s := NewShard(shardID, pkg)
	sr.shards[pkg] = s
	return s
}

// RouteShards selects target shards according to localized primary and secondary scope.
func (sr *ShardRouter) RouteShards(primaryScope, secondaryScope []string) []*Shard {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	if len(sr.shards) == 0 {
		return []*Shard{sr.defaultShard}
	}

	// If no specific scope, search all shards
	if len(primaryScope) == 0 && len(secondaryScope) == 0 {
		all := make([]*Shard, 0, len(sr.shards))
		for _, s := range sr.shards {
			all = append(all, s)
		}
		return all
	}

	matched := make(map[string]*Shard)

	// Check primary scope
	for _, p := range primaryScope {
		if s, ok := sr.shards[p]; ok {
			matched[s.ID] = s
		}
	}

	// Check secondary scope if few matched
	if len(matched) == 0 {
		for _, s := range secondaryScope {
			if sh, ok := sr.shards[s]; ok {
				matched[sh.ID] = sh
			}
		}
	}

	// Fallback to all shards if none matched
	if len(matched) == 0 {
		all := make([]*Shard, 0, len(sr.shards))
		for _, s := range sr.shards {
			all = append(all, s)
		}
		return all
	}

	res := make([]*Shard, 0, len(matched))
	for _, s := range matched {
		res = append(res, s)
	}
	return res
}

// ShardSearchResult encapsulates candidates and pruning stats from a shard search.
type ShardSearchResult struct {
	ShardID string
	Results []ScoredDoc
	Stats   PruningStats
}

// SearchAcrossShards executes bounded parallel search across target shards using Block-Max WAND.
func (sr *ShardRouter) SearchAcrossShards(
	terms []string,
	targetShards []*Shard,
	k int,
) ([]ShardSearchResult, PruningStats) {
	if len(targetShards) == 0 || len(terms) == 0 || k <= 0 {
		return nil, PruningStats{}
	}

	results := make([]ShardSearchResult, len(targetShards))
	var totalStats PruningStats
	var statsMu sync.Mutex

	// Bounded worker pool channel semaphore
	sem := make(chan struct{}, sr.maxConcurrency)
	var wg sync.WaitGroup

	for i, sh := range targetShards {
		wg.Add(1)
		go func(idx int, target *Shard) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			// Collect iterators for each term in this shard
			var iterators []*PostingIterator
			for _, term := range terms {
				iters := target.SegmentMgr.SearchTerm(term)
				iterators = append(iterators, iters...)
			}

			if len(iterators) == 0 {
				return
			}

			scored, stats := BlockMaxWAND(iterators, k)

			results[idx] = ShardSearchResult{
				ShardID: target.ID,
				Results: scored,
				Stats:   stats,
			}

			statsMu.Lock()
			totalStats.PostingsVisited += stats.PostingsVisited
			totalStats.BlocksSkipped += stats.BlocksSkipped
			totalStats.SuperblocksSkipped += stats.SuperblocksSkipped
			totalStats.CandidatesScored += stats.CandidatesScored
			statsMu.Unlock()
		}(i, sh)
	}

	wg.Wait()
	return results, totalStats
}
