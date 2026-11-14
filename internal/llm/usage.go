package llm

import (
	"unicode"
	"unicode/utf8"
)

// EstimateTokens is a rough token count for s when a backend reports
// none: about one token per CJK character and per four bytes of other text.
func EstimateTokens(s string) int {
	n, rest := 0, 0
	for _, r := range s {
		if r >= 0x2E80 && (unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r)) {
			n++
			continue
		}
		rest += utf8.RuneLen(r)
	}
	return n + (rest+3)/4
}

func estimateUsage(msgs []Message, text string) Usage {
	p := 0
	for _, m := range msgs {
		p += EstimateTokens(m.Content) + 4
	}
	c := EstimateTokens(text)
	return Usage{PromptTokens: p, CompletionTokens: c, TotalTokens: p + c, Estimated: true}
}
