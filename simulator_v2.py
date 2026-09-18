from __future__ import annotations

from dataclasses import dataclass
from typing import List, Set, Tuple
import random

@dataclass(frozen=True)
class Candidate:
    name: str
    tokens: int
    semantic: float
    graph: float
    freshness: float
    authority: float
    reuse: float
    task_affinity: float
    risk: float
    required: bool = False

    def score(self) -> float:
        # Hard safety filtering happens before this score.
        return (1.0*self.semantic + 1.2*self.graph + 0.8*self.freshness
                + 1.0*self.authority + 0.8*self.reuse + 0.8*self.task_affinity
                - 0.3*(self.tokens/1000))


def safe_candidates(cs: List[Candidate], min_authority=0.65, max_risk=0.20) -> List[Candidate]:
    return [c for c in cs if c.authority >= min_authority and c.risk <= max_risk]


def greedy(cs: List[Candidate], budget: int) -> List[Candidate]:
    chosen, used = [], 0
    # marginal proxy = score / tokens; deterministic baseline
    for c in sorted(cs, key=lambda x: x.score()/max(x.tokens,1), reverse=True):
        if c.score() > 0 and used + c.tokens <= budget:
            chosen.append(c); used += c.tokens
    return chosen


def topk_semantic(cs: List[Candidate], budget: int) -> List[Candidate]:
    chosen, used = [], 0
    for c in sorted(cs, key=lambda x: x.semantic, reverse=True):
        if used + c.tokens <= budget:
            chosen.append(c); used += c.tokens
    return chosen


def full(cs: List[Candidate], budget: int) -> List[Candidate]:
    return [c for c in cs if c.tokens <= budget]


def outcome(chosen: List[Candidate], required: Set[str], rng: random.Random) -> bool:
    names = {c.name for c in chosen}
    coverage = len(names & required) / len(required)
    # Noise penalty approximates distraction from irrelevant / stale context.
    noise = sum(max(0.0, 1.0-c.task_affinity) for c in chosen)
    # Saturating task-success model: full required coverage is necessary but not strictly sufficient.
    p = max(0.02, min(0.99, 0.15 + 0.78*coverage - 0.06*noise))
    return rng.random() < p


def run(n=5000, seed=7):
    rng = random.Random(seed)
    totals = {k: [0,0,0] for k in ("full","semantic","asc")}
    # 100 synthetic tasks per difficulty bucket; candidate structure is varied.
    for _ in range(n):
        req_names = {"current_symbol", "decision", "test"}
        cs = [
            Candidate("current_symbol", 500, .84,.96,1,.98,.7,.96,.02,True),
            Candidate("decision", 180, .80,.91,.88,1,.91,.93,.03,True),
            Candidate("test", 350, .72,.84,1,1,.7,.87,.02,True),
            Candidate("failure", 140, .76,.86,.90,.94,.82,.91,.04),
            Candidate("repo_map", 900, .77,.93,.98,.90,.82,.78,.07),
            Candidate("old_narrative", 1800, .89,.42,.20,.55,.35,.54,.58),
            Candidate("unrelated_docs", 2500, .86,.08,.98,.82,.20,.15,.05),
            Candidate("same_name_other_repo", 600, .88,.11,.99,.84,.50,.25,.35),
        ]
        # Randomly perturb relevance to mimic task diversity.
        cs = [Candidate(c.name,c.tokens,
                        max(0,min(1,c.semantic+rng.gauss(0,.03))),
                        max(0,min(1,c.graph+rng.gauss(0,.03))),
                        c.freshness,c.authority,c.reuse,c.task_affinity,c.risk,c.required)
              for c in cs]
        budget = rng.choice([1000,1500,2000,3000,4000])
        candidates = cs
        for label, selected in (
            ("full", full(candidates,budget)),
            ("semantic", topk_semantic(candidates,budget)),
            ("asc", greedy(safe_candidates(candidates),budget)),
        ):
            totals[label][0] += int(outcome(selected,req_names,rng))
            totals[label][1] += sum(c.tokens for c in selected)
            totals[label][2] += 1
    for label,(succ,toks,count) in totals.items():
        print(f"{label}: success={succ/count:.3f} avg_tokens={toks/count:.1f} efficiency={(succ/count)/(toks/count)*1000:.3f} success/k-token")

if __name__ == "__main__":
    run()
