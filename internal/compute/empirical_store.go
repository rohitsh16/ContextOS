package compute

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// JSONLObservationStore is append-only so benchmark evidence cannot be silently rewritten.
type JSONLObservationStore struct{ Path string }

func (s JSONLObservationStore) Append(obs EmpiricalObservation) error {
	if s.Path == "" {
		return fmt.Errorf("observation path is required")
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(obs)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}
func (s JSONLObservationStore) Load() ([]EmpiricalObservation, error) {
	f, err := os.Open(s.Path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []EmpiricalObservation
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var o EmpiricalObservation
		if err := json.Unmarshal(sc.Bytes(), &o); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, sc.Err()
}
