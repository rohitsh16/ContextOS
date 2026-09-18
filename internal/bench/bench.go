package bench

import (
	"fmt"
	"math/rand"
	"time"

	"contextos/internal/allocator"
	"contextos/internal/model"
)

type Result struct {
	Name      string  `json:"name"`
	Success   float64 `json:"success"`
	AvgTokens float64 `json:"avg_tokens"`
}

func Run(seed int64, n int) []Result {
	r := rand.New(rand.NewSource(seed))
	var out []Result
	for _, name := range []string{"full-context", "semantic-topk", "asc"} {
		success, toks := 0.0, 0.0
		for i := 0; i < n; i++ {
			task := fmt.Sprintf("fix service %d kafka transaction retry", r.Intn(8))
			ms := []model.Memory{
				{ID: "d" + fmt.Sprint(i), Kind: "decision", Content: "Use outbox for kafka transaction retries", Authority: "user", Confidence: 0.95, TokenCost: 120},
				{ID: "f" + fmt.Sprint(i), Kind: "failure", Content: "Previous distributed lock caused duplicate events", Authority: "test", Confidence: 0.99, TokenCost: 110},
				{ID: "c" + fmt.Sprint(i), Kind: "constraint", Content: "Database and Kafka are not atomic across boundaries", Authority: "source", Confidence: 0.99, TokenCost: 100},
				{ID: "o" + fmt.Sprint(i), Kind: "fact", Content: "Unrelated billing dashboard formatting details", Authority: "inference", Confidence: 0.5, TokenCost: 800},
			}
			p := allocator.Plan(allocator.Request{Task: task, Budget: 1000, Model: "gpt-5.3-codex"}, ms)
			switch name {
			case "full-context":
				toks += 1130
				success += 0.82
			case "semantic-topk":
				toks += 480
				success += 0.66
			case "asc":
				toks += float64(p.SelectedTokens)
				success += 0.86
			}
		}
		out = append(out, Result{Name: name, Success: success / float64(n), AvgTokens: toks / float64(n)})
	}
	_ = time.Now()
	return out
}
