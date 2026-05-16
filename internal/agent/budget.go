package agent

const (
	defaultComposeTokenBudget = 4000
	maxComposeTokenBudget     = 12000
	minWriterMaxTokens        = 2048
	maxWriterMaxTokens        = 16000
)

// RecommendedComposeTokenBudget scales deterministic context assembly with the
// requested chapter size while keeping a conservative ceiling.
func RecommendedComposeTokenBudget(targetWords int) int {
	if targetWords <= 0 {
		return defaultComposeTokenBudget
	}
	if targetWords < defaultComposeTokenBudget {
		return defaultComposeTokenBudget
	}
	if targetWords > maxComposeTokenBudget {
		return maxComposeTokenBudget
	}
	return targetWords
}

// RecommendedWriterMaxTokens gives the writer enough output headroom to reach
// longer chapter targets without asking for unbounded completions.
func RecommendedWriterMaxTokens(spec LengthSpec) int {
	target := spec.Target
	if target <= 0 {
		target = spec.Max
	}
	if target <= 0 {
		target = spec.Min
	}
	budget := target * 3 / 2
	if budget < spec.Max {
		budget = spec.Max
	}
	if budget < minWriterMaxTokens {
		budget = minWriterMaxTokens
	}
	if budget > maxWriterMaxTokens {
		budget = maxWriterMaxTokens
	}
	return budget
}
