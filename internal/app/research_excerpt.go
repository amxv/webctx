package app

import (
	"sort"
	"strings"
)

const backtickFence = "\x60\x60\x60"

type evidenceBlock struct {
	text  string
	code  bool
	score int
	order int
}

// A code fence is indivisible evidence: do not silently truncate a command,
// request body or SDK example and then invent a closing fence.
func splitEvidenceBlocks(markdown, question string) []evidenceBlock {
	var blocks []evidenceBlock
	var content strings.Builder
	terms := queryTerms(question)
	inCode := false
	fence := ""
	flush := func(code bool) {
		text := strings.TrimSpace(content.String())
		content.Reset()
		if text == "" {
			return
		}
		score := termScore(text, terms)
		if code {
			score *= 3
			if strings.Contains(strings.ToLower(question), "rest") &&
				(strings.Contains(text, "curl") || strings.Contains(text, "POST ") || strings.Contains(text, "GET ")) {
				score += 4
			}
		}
		blocks = append(blocks, evidenceBlock{text: text, code: code, score: score, order: len(blocks)})
	}
	for _, line := range strings.Split(markdown, "\n") {
		trimmed := strings.TrimSpace(line)
		opening := !inCode && (strings.HasPrefix(trimmed, backtickFence) || strings.HasPrefix(trimmed, "~~~"))
		if opening {
			flush(false)
			inCode = true
			fence = trimmed[:3]
		}
		content.WriteString(line)
		content.WriteByte('\n')
		if inCode && !opening && strings.TrimSpace(strings.TrimPrefix(trimmed, fence)) == "" &&
			strings.HasPrefix(trimmed, fence) {
			flush(true)
			inCode = false
			fence = ""
		}
	}
	// Unclosed code blocks are not valid examples; treat them as source prose.
	flush(false)
	return blocks
}

// Select complete, high-relevance examples and explanatory prose from long
// sections, preserving original order and the hard context budget.
func atomicFocusedSection(markdown, question string, budget int) string {
	markdown = strings.TrimSpace(markdown)
	if len(markdown) <= budget {
		return markdown
	}
	if budget < 180 {
		return "*[Section exceeds excerpt budget; follow the source URL.]*"
	}
	blocks := splitEvidenceBlocks(markdown, question)
	const omitted = "\n\n*[Other source text omitted. Follow the original URL for full details.]*"
	remaining := budget - len(omitted) - 8
	if remaining < 80 {
		return "*[Section exceeds excerpt budget; follow the source URL.]*"
	}
	selected := make(map[int]string)
	var codes []evidenceBlock
	var prose []evidenceBlock
	for _, block := range blocks {
		if block.code {
			codes = append(codes, block)
		} else {
			prose = append(prose, block)
		}
	}
	sort.SliceStable(codes, func(i, j int) bool {
		if codes[i].score == codes[j].score {
			return codes[i].order < codes[j].order
		}
		return codes[i].score > codes[j].score
	})
	// Reserve a little space for the section explanation.
	codeAllowance := remaining * 85 / 100
	for _, block := range codes {
		size := len(block.text) + 2
		if size <= codeAllowance {
			selected[block.order] = block.text
			codeAllowance -= size
			remaining -= size
		}
	}
	sort.SliceStable(prose, func(i, j int) bool {
		if prose[i].score == prose[j].score {
			return prose[i].order < prose[j].order
		}
		return prose[i].score > prose[j].score
	})
	for _, block := range prose {
		if remaining < 100 {
			break
		}
		if len(block.text)+2 <= remaining {
			selected[block.order] = block.text
			remaining -= len(block.text) + 2
		} else if part := completeProseLines(block.text, remaining-4); part != "" {
			selected[block.order] = part
			remaining -= len(part) + 2
		}
	}
	var output []string
	for _, block := range blocks {
		if text := selected[block.order]; text != "" {
			output = append(output, text)
		}
	}
	if len(output) == 0 {
		return "*[Code examples exceed the excerpt budget; inspect source URL for the full examples.]*"
	}
	return strings.Join(output, "\n\n") + omitted
}

func completeProseLines(text string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	var lines []string
	size := 0
	for _, line := range strings.Split(text, "\n") {
		if size+len(line)+1 > maxBytes {
			break
		}
		lines = append(lines, line)
		size += len(line) + 1
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
