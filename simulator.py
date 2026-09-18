from __future__ import annotations

from dataclasses import dataclass
from typing import List, Tuple

@dataclass
class Candidate:
    name: str
    tokens: int
    semantic: float
    graph: float
    freshness: float
    authority: float
    reuse: float
    task_affinity: float
    stale_risk: float = 0.0

    def score(self) -> float:
        return (
            1.0*self.semantic + 1.2*self.graph + 0.8*self.freshness
            + 1.0*self.authority + 0.8*self.reuse + 0.8*self.task_affinity
            - 1.0*self.stale_risk
            - 0.3*(self.tokens/1000.0)
        )

    def density(self) -> float:
        return self.score() / max(self.tokens, 1)


def greedy_select(candidates: List[Candidate], budget: int) -> Tuple[List[Candidate], int]:
    chosen: List[Candidate] = []
    used = 0
    for c in sorted(candidates, key=lambda x: x.density(), reverse=True):
        if c.score() <= 0:
            continue
        if used + c.tokens <= budget:
            chosen.append(c)
            used += c.tokens
    return chosen, used


def main() -> None:
    candidates = [
        Candidate("current_symbol", 500, .82, .95, 1.0, 1.0, .75, .95),
        Candidate("architecture_decision", 180, .78, .90, .85, 1.0, .90, .92),
        Candidate("previous_failure", 140, .74, .88, .88, .95, .82, .90),
        Candidate("old_design_discussion", 1800, .87, .45, .25, .55, .25, .50, stale_risk=.45),
        Candidate("unrelated_docs", 2500, .85, .10, .95, .80, .20, .15),
        Candidate("test_evidence", 350, .70, .82, 1.0, 1.0, .70, .84),
        Candidate("repo_map", 900, .76, .92, .98, .90, .82, .78),
    ]
    for budget in (1000, 2000, 4000):
        chosen, used = greedy_select(candidates, budget)
        print(f"budget={budget} used={used}")
        print("  " + ", ".join(f"{c.name}:{c.tokens}" for c in chosen))

if __name__ == "__main__":
    main()
