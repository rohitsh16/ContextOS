package compute

import (
	"path/filepath"
	"testing"
)

func TestJSONLObservationStoreRoundTrip(t *testing.T) {
	s := JSONLObservationStore{Path: filepath.Join(t.TempDir(), "observations.jsonl")}
	want := EmpiricalObservation{TaskID: "t1", TaskFamily: "concurrency", Provider: "openai", Model: "o3-mini", Effort: EffortMedium, Success: true}
	if err := s.Append(want); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load()
	if err != nil || len(got) != 1 || got[0].TaskFamily != want.TaskFamily {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
