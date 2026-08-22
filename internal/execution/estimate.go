package execution

// EstimateTokens returns a conservative char/4 token estimate, the same
// heuristic pi uses to decide compaction before calling the model.
func EstimateTokens(text string) int64 {
	if text == "" {
		return 0
	}
	chars := 0
	for range text {
		chars++
	}
	estimate := chars / 4
	if chars%4 != 0 {
		estimate++
	}
	return int64(estimate)
}
