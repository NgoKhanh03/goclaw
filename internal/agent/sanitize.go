// Package agent — response sanitization pipeline.
//
// Matching TS sanitization chain:
//
//	extractAssistantText() → per-block:
//	  1. stripMinimaxToolCallXml()        → Go: stripGarbledToolXML()
//	  2. stripDowngradedToolCallText()     → Go: stripDowngradedToolCallText()
//	  3. stripThinkingTagsFromText()       → Go: stripThinkingTags()
//	  then:
//	  4. sanitizeUserFacingText()          → Go: sanitizeUserFacingText()
//	     - stripFinalTagsFromText()        → Go: stripFinalTags()
//	     - collapseConsecutiveDuplicateBlocks()
//
// Additional Go-specific:
//	  5. stripEchoedSystemMessages()       → strip hallucinated [System Message] blocks
//	  6. stripGarbledToolXML()             → strip garbled XML from models like DeepSeek
package agent

import (
	"log/slog"
	"regexp"
	"strings"
)

// SanitizeAssistantContent applies the full sanitization pipeline to assistant
// response text before saving to session and sending to user.
// Matching TS extractAssistantText() + sanitizeUserFacingText().
func SanitizeAssistantContent(content string) string {
	if content == "" {
		return content
	}

	original := content

	// 1. Strip garbled tool-call XML (DeepSeek, GLM, Minimax)
	content = stripGarbledToolXML(content)
	if content == "" {
		return ""
	}

	// 2. Strip downgraded tool call text ([Tool Call: ...], [Tool Result ...])
	content = stripDowngradedToolCallText(content)

	// 3. Strip thinking/reasoning tags (<think>, <thinking>, <thought>, <antThinking>)
	content = stripThinkingTags(content)

	// 4. Strip <final> tags (keep content inside)
	content = stripFinalTags(content)

	// 5. Strip echoed [System Message] blocks
	content = stripEchoedSystemMessages(content)

	// 6. Collapse consecutive duplicate blocks
	content = collapseConsecutiveDuplicateBlocks(content)

	// 7. Strip MEDIA: paths from LLM output (media delivered separately)
	content = stripMediaPaths(content)

	// 8. Strip leading blank lines (preserve indentation)
	content = stripLeadingBlankLines(content)

	content = strings.TrimSpace(content)

	if content != original {
		slog.Debug("sanitized assistant content",
			"original_len", len(original),
			"cleaned_len", len(content),
		)
	}

	return content
}

// --- 1. Garbled tool-call XML ---

// garbledToolXMLBlockPattern matches entire tool_call/tool_use/function_calls/function_response blocks
// including their content (proxy may embed tool execution inline in response content).
// Also catches antml:function_calls / antml:invoke / antml:parameter variants.
var garbledToolXMLBlockPattern = regexp.MustCompile(
	`(?si)\s*<(?:antml:)?(?:tool_call|tool_use|function_calls?|function_response|invoke|invfunction_calls|functioninvoke|minimax:tool_call|parameter)[^>]*>.*?</(?:antml:)?(?:tool_call|tool_use|function_calls?|function_response|invoke|invfunction_calls|functioninvoke|minimax:tool_call|parameter)>`,
)

// toolResponseBlockPattern matches <tool_response>...</tool_response> and
// <function_response>...</function_response> blocks that proxies embed
// when they execute tools inline.
var toolResponseBlockPattern = regexp.MustCompile(
	`(?si)\s*<(?:antml:)?(?:tool_response|function_response)[^>]*>.*?</(?:antml:)?(?:tool_response|function_response)>`,
)

// strayClosingTagPattern matches orphan closing tags that appear when
// a partial tool-call block is streamed across multiple chunks, e.g.
// "Successfully edited IDENTITY.md </function_response> </function_calls>".
var strayClosingTagPattern = regexp.MustCompile(
	`(?i)\s*</(?:antml:)?(?:tool_call|tool_use|function_calls?|function_response|tool_response|invoke|invfunction_calls|functioninvoke|minimax:tool_call|parameter)>`,
)

// strayOpeningTagPattern matches opening tags without proper close (partial XML).
var strayOpeningTagPattern = regexp.MustCompile(
	`(?i)<(?:antml:)?(?:tool_call|tool_use|function_calls?|function_response|tool_response|invoke|invfunction_calls|functioninvoke|minimax:tool_call|parameter)\b[^>]*>`,
)

var garbledToolXMLIndicators = []string{
	"invfunction_calls",
	"functioninvoke",
	"<parameter name=",
	"</parameter",
	"<function_call",
	"<function_response",
	"</function_response",
	"</function_calls",
	"<tool_call",
	"<tool_use",
	"<tool_response",
	"<minimax:tool_call",
	"<function_calls",
	"<invoke",
	"<parameter",
	"antml:function_calls",
}

func stripGarbledToolXML(content string) string {
	hasIndicator := false
	lower := strings.ToLower(content)
	for _, ind := range garbledToolXMLIndicators {
		if strings.Contains(lower, strings.ToLower(ind)) {
			hasIndicator = true
			break
		}
	}
	if !hasIndicator {
		return content
	}

	// Strip entire blocks (including inner content) so proxy-embedded tool
	// executions are removed while keeping the surrounding user-facing text.
	cleaned := garbledToolXMLBlockPattern.ReplaceAllString(content, "")
	cleaned = toolResponseBlockPattern.ReplaceAllString(cleaned, "")
	// Also strip any orphaned opening/closing tags left over from partial blocks.
	cleaned = strayClosingTagPattern.ReplaceAllString(cleaned, "")
	cleaned = strayOpeningTagPattern.ReplaceAllString(cleaned, "")
	cleaned = strings.TrimSpace(cleaned)

	if cleaned != "" {
		slog.Warn("stripped inline tool call blocks from proxy response",
			"original_len", len(content),
			"remaining_len", len(cleaned),
		)
		return cleaned
	}

	// Nothing left after strip → entire response was tool XML
	slog.Warn("stripped entire response as garbled tool XML", "original_len", len(content))
	return ""
}

// --- 2. Downgraded tool call text ---

// stripDowngradedToolCallText removes [Tool Call: ...], [Tool Result ...],
// and [Historical context: ...] blocks that some models emit as text.
// Matching TS stripDowngradedToolCallText().
// Uses line-by-line scanning (Go regexp doesn't support lookahead).
func stripDowngradedToolCallText(content string) string {
	if !strings.Contains(content, "[Tool Call:") &&
		!strings.Contains(content, "[Tool Result") &&
		!strings.Contains(content, "[Historical context:") {
		return content
	}

	lines := strings.Split(content, "\n")
	var result []string
	skipping := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Start skipping on these markers
		if strings.HasPrefix(trimmed, "[Tool Call:") ||
			strings.HasPrefix(trimmed, "[Tool Result") ||
			strings.HasPrefix(trimmed, "[Historical context:") {
			skipping = true
			continue
		}

		// Stop skipping on non-indented, non-empty line that isn't part of the block
		if skipping {
			// Arguments JSON and tool output are typically indented or empty
			if trimmed == "" || strings.HasPrefix(trimmed, "Arguments:") ||
				strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "}") {
				continue
			}
			// Non-tool-block line → stop skipping
			skipping = false
		}

		result = append(result, line)
	}

	return strings.TrimSpace(strings.Join(result, "\n"))
}

// --- 3. Thinking/reasoning tags ---

// Matches TS stripThinkingTagsFromText() with strict mode.
// Strips: <think>...</think>, <thinking>...</thinking>, <thought>...</thought>,
//         <antThinking>...</antThinking>
// Go regexp doesn't support backreferences, so we use separate patterns.
var thinkingTagPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?is)<think>.*?</think>`),
	regexp.MustCompile(`(?is)<thinking>.*?</thinking>`),
	regexp.MustCompile(`(?is)<thought>.*?</thought>`),
	regexp.MustCompile(`(?is)<antThinking>.*?</antThinking>`),
	regexp.MustCompile(`(?is)<antthinking>.*?</antthinking>`),
}

func stripThinkingTags(content string) string {
	lower := strings.ToLower(content)
	if !strings.Contains(lower, "<think") && !strings.Contains(lower, "<thought") &&
		!strings.Contains(lower, "<antthinking") {
		return content
	}
	result := content
	for _, pat := range thinkingTagPatterns {
		result = pat.ReplaceAllString(result, "")
	}
	return strings.TrimSpace(result)
}

// --- 4. <final> tags ---

// Matches TS stripFinalTagsFromText(). Removes <final> and </final> tags
// but keeps the content inside.
var finalTagPattern = regexp.MustCompile(`(?i)<\s*/?\s*final\s*>`)

func stripFinalTags(content string) string {
	if !strings.Contains(strings.ToLower(content), "final") {
		return content
	}
	return finalTagPattern.ReplaceAllString(content, "")
}

// --- 5. Echoed [System Message] ---

// stripEchoedSystemMessages removes "[System Message] ..." blocks that LLMs
// hallucinate/echo in their response text.
// Uses line-based scanning (Go regexp doesn't support lookahead).
func stripEchoedSystemMessages(content string) string {
	if !strings.Contains(content, "[System Message]") {
		return content
	}

	lines := strings.Split(content, "\n")
	var result []string
	skipping := false

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "[System Message]") {
			skipping = true
			continue
		}
		if skipping {
			// Empty line ends the system message block
			if strings.TrimSpace(line) == "" {
				skipping = false
				continue
			}
			// Still part of the system message block (Stats:, reply instructions, etc.)
			continue
		}
		result = append(result, line)
	}

	cleaned := strings.TrimSpace(strings.Join(result, "\n"))

	if cleaned != strings.TrimSpace(content) {
		slog.Warn("stripped echoed [System Message] from assistant response",
			"original_len", len(content),
			"cleaned_len", len(cleaned),
		)
	}

	return cleaned
}

// --- 6. Collapse consecutive duplicate blocks ---

// collapseConsecutiveDuplicateBlocks removes repeated paragraph blocks.
// Matching TS collapseConsecutiveDuplicateBlocks().
func collapseConsecutiveDuplicateBlocks(content string) string {
	blocks := strings.Split(content, "\n\n")
	if len(blocks) <= 1 {
		return content
	}

	var result []string
	for i, block := range blocks {
		trimmed := strings.TrimSpace(block)
		if trimmed == "" {
			continue
		}
		if i > 0 && len(result) > 0 && trimmed == strings.TrimSpace(result[len(result)-1]) {
			continue // skip duplicate
		}
		result = append(result, block)
	}

	collapsed := strings.Join(result, "\n\n")
	if collapsed != content {
		slog.Debug("collapsed duplicate blocks",
			"original_blocks", len(blocks),
			"result_blocks", len(result),
		)
	}
	return collapsed
}

// --- 7. Strip MEDIA: paths ---

// stripMediaPaths removes lines containing MEDIA:/path references from LLM output.
// These are tool result artifacts that should not appear in user-facing text
// (media files are delivered separately via OutboundMessage.Media).
func stripMediaPaths(content string) string {
	if !strings.Contains(content, "MEDIA:") {
		return content
	}
	lines := strings.Split(content, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[[audio_as_voice]]") {
			continue
		}
		// Strip any line containing a MEDIA: path reference, regardless of wrapping format.
		// LLMs echo these in many forms: bare "MEDIA:/path", markdown "![alt](MEDIA:/path)",
		// JSON '{"image":"MEDIA:/path"}', etc. The /tmp/ or / after MEDIA: confirms it's a path.
		if strings.Contains(trimmed, "MEDIA:/") {
			continue
		}
		result = append(result, line)
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

// --- 8. Strip leading blank lines ---

var leadingBlankLinesPattern = regexp.MustCompile(`^(?:[ \t]*\r?\n)+`)

func stripLeadingBlankLines(content string) string {
	return leadingBlankLinesPattern.ReplaceAllString(content, "")
}

// --- Stream chunk sanitization ---

// toolCallXMLStreamIndicators are substrings that identify tool-call XML
// fragments in a streaming chunk. When detected, the chunk content is scrubbed
// before emitting to the client (prevents raw XML flashing in the UI).
var toolCallXMLStreamIndicators = []string{
	"</function_calls",
	"</function_response",
	"<function_calls",
	"<function_response",
	"</tool_call",
	"</tool_use",
	"</tool_response",
	"<tool_call",
	"<tool_use",
	"<tool_response",
	"<invoke",
	"<parameter name=",
	"</parameter>",
	"invfunction_calls",
	"functioninvoke",
	"antml:function_calls",
	"antml:invoke",
	"antml:parameter",
}

// hasToolCallXMLIndicator checks if text contains any tool-call XML indicator.
func hasToolCallXMLIndicator(text string) bool {
	lower := strings.ToLower(text)
	for _, ind := range toolCallXMLStreamIndicators {
		if strings.Contains(lower, strings.ToLower(ind)) {
			return true
		}
	}
	return false
}

// SanitizeStreamChunk performs lightweight sanitization on a streaming content
// chunk. This is called in real-time as chunks arrive (before emitting to UI)
// to prevent raw tool-call XML from flashing in the chat.
//
// This is NOT a replacement for the full SanitizeAssistantContent pipeline —
// that still runs on the final accumulated content. This function handles the
// most critical case: tool-call XML appearing in streamed text.
func SanitizeStreamChunk(chunk string) string {
	if chunk == "" {
		return chunk
	}
	if !hasToolCallXMLIndicator(chunk) {
		return chunk
	}

	cleaned := garbledToolXMLBlockPattern.ReplaceAllString(chunk, "")
	cleaned = toolResponseBlockPattern.ReplaceAllString(cleaned, "")
	cleaned = strayClosingTagPattern.ReplaceAllString(cleaned, "")
	cleaned = strayOpeningTagPattern.ReplaceAllString(cleaned, "")

	if cleaned != chunk {
		slog.Debug("sanitized stream chunk",
			"original_len", len(chunk),
			"cleaned_len", len(cleaned),
		)
	}
	return cleaned
}

// --- NO_REPLY detection ---

// IsSilentReply checks if the text is a NO_REPLY token.
// Matching TS isSilentReplyText() from auto-reply/tokens.ts.
func IsSilentReply(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	const token = "NO_REPLY"
	// Exact match
	if trimmed == token {
		return true
	}
	// Starts with token followed by non-word char or end
	if strings.HasPrefix(trimmed, token) {
		rest := trimmed[len(token):]
		if rest == "" || !isWordChar(rune(rest[0])) {
			return true
		}
	}
	// Ends with token preceded by non-word char
	if strings.HasSuffix(trimmed, token) {
		before := trimmed[:len(trimmed)-len(token)]
		if before == "" || !isWordChar(rune(before[len(before)-1])) {
			return true
		}
	}
	return false
}

func isWordChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
}
