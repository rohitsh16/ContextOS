package evaluator

import "testing"

func TestPolicyLabelCannotChangeScore(t *testing.T) {
	c := .1
	m := Manifest{QualityFloor: .9}
	tasks := []Task{{ID: "t", Critical: true}}
	a := Execution{RunID: "a", TaskID: "t", Policy: "ContextOS", Cost: &c, Quality: 1, Success: true}
	b := a
	b.RunID = "b"
	b.Policy = "baseline"
	s, e := ScoreAll(m, tasks, []Execution{a, b})
	if e != nil || !s[0].FloorPass || !s[1].FloorPass {
		t.Fatalf("policy leaked into evaluator: %+v %v", s, e)
	}
}
