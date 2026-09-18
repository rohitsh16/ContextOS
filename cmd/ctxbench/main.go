package main

import (
	"contextos/internal/bench"
	"encoding/json"
	"flag"
	"fmt"
)

func main() {
	seed := flag.Int64("seed", 42, "seed")
	n := flag.Int("n", 1000, "synthetic tasks")
	flag.Parse()
	r := bench.Run(*seed, *n)
	b, _ := json.MarshalIndent(r, "", "  ")
	fmt.Println(string(b))
}
