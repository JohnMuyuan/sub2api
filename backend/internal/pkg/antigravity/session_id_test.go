package antigravity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"regexp"
	"strconv"
	"sync"
	"testing"
)

var sessionIDPattern = regexp.MustCompile(`^-\d+$`)

func legacySessionIDForText(text string) string {
	h := sha256.Sum256([]byte(text))
	n := int64(binary.BigEndian.Uint64(h[:8])) & 0x7FFFFFFFFFFFFFFF
	return "-" + strconv.FormatInt(n, 10)
}

func userText(text string) []GeminiContent {
	return []GeminiContent{{Role: "user", Parts: []GeminiPart{{Text: text}}}}
}

func TestGenerateStableSessionID_Table(t *testing.T) {
	imageThenText := []GeminiContent{{Role: "user", Parts: []GeminiPart{
		{InlineData: &GeminiInlineData{MimeType: "image/png", Data: "AAAA"}},
		{Text: "describe"},
	}}}
	imageOnly := []GeminiContent{{Role: "user", Parts: []GeminiPart{
		{InlineData: &GeminiInlineData{MimeType: "image/png", Data: "AAAA"}},
	}}}

	tests := []struct {
		name     string
		a, b     []GeminiContent
		scopeA   string
		scopeB   string
		wantSame bool
	}{
		{"same opener same scope is stable", userText("hi"), userText("hi"), "account:1|api_key:1", "account:1|api_key:1", true},
		{"same opener different api key differs", userText("hi"), userText("hi"), "account:1|api_key:1", "account:1|api_key:2", false},
		{"same opener different account differs", userText("hi"), userText("hi"), "account:1|api_key:1", "account:2|api_key:1", false},
		{"different opener same scope differs", userText("hi"), userText("hello"), "s", "s", false},
		{"text after image is used deterministically", imageThenText, imageThenText, "s", "s", true},
		{"image-only first message is deterministic", imageOnly, imageOnly, "s", "s", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := generateStableSessionID(tt.a, tt.scopeA)
			b := generateStableSessionID(tt.b, tt.scopeB)
			if !sessionIDPattern.MatchString(a) || !sessionIDPattern.MatchString(b) {
				t.Fatalf("unexpected format: %q %q", a, b)
			}
			if (a == b) != tt.wantSame {
				t.Fatalf("same=%v, want %v (a=%s b=%s)", a == b, tt.wantSame, a, b)
			}
		})
	}
}

func TestGenerateStableSessionID_EmptyScopeKeepsLegacyAlgorithm(t *testing.T) {
	if got, want := generateStableSessionID(userText("hello world"), ""), legacySessionIDForText("hello world"); got != want {
		t.Fatalf("legacy compatibility broken: got %s want %s", got, want)
	}
}

func TestGenerateStableSessionID_NoUserContentIsUnique(t *testing.T) {
	contents := []GeminiContent{{Role: "model", Parts: []GeminiPart{{Text: "x"}}}}
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := generateStableSessionID(contents, "s")
		if !sessionIDPattern.MatchString(id) {
			t.Fatalf("unexpected format %q", id)
		}
		if seen[id] {
			t.Fatalf("random fallback collided: %s", id)
		}
		seen[id] = true
	}
}

func TestResolveSessionID_MetadataUserID(t *testing.T) {
	const raw = "user_abc_account_123_session_456"
	if got := resolveSessionID(userText("hi"), raw, ""); got != raw {
		t.Fatalf("without scope metadata.user_id should pass through, got %q", got)
	}
	scoped := resolveSessionID(userText("hi"), raw, "account:1|api_key:1")
	if scoped == raw || !sessionIDPattern.MatchString(scoped) {
		t.Fatalf("scoped metadata.user_id must be hashed into -<digits>, got %q", scoped)
	}
	if again := resolveSessionID(userText("other"), raw, "account:1|api_key:1"); again != scoped {
		t.Fatalf("metadata.user_id session must be stable regardless of content: %s vs %s", again, scoped)
	}
	if other := resolveSessionID(userText("hi"), raw, "account:2|api_key:1"); other == scoped {
		t.Fatal("metadata.user_id session must differ across accounts")
	}
}

// 请求结构快照：除 requestId 外，相同输入 + 相同配置应产生字节级一致的请求体。
func TestTransformClaudeToGemini_DeterministicExceptRequestID(t *testing.T) {
	req := &ClaudeRequest{
		Model:     "claude-sonnet-4-5",
		MaxTokens: 1024,
		System:    json.RawMessage(`"be brief"`),
		Messages: []ClaudeMessage{
			{Role: "user", Content: json.RawMessage(`"hi"`)},
		},
		Tools: []ClaudeTool{{Name: "get_weather", Description: "w", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"city": map[string]any{"type": "string"}, "unit": map[string]any{"type": "string"}},
		}}},
	}
	opts := DefaultTransformOptions()
	opts.SessionScope = "account:7|api_key:9"

	normalize := func() map[string]any {
		body, err := TransformClaudeToGeminiWithOptions(req, "proj-1", "claude-sonnet-4-5", opts)
		if err != nil {
			t.Fatalf("transform: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(body, &m); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		rid, _ := m["requestId"].(string)
		if !regexp.MustCompile(`^agent-[0-9a-f-]{36}$`).MatchString(rid) {
			t.Fatalf("requestId must be unique agent-<uuid>, got %q", rid)
		}
		delete(m, "requestId")
		return m
	}
	first, _ := json.Marshal(normalize())
	second, _ := json.Marshal(normalize())
	if string(first) != string(second) {
		t.Fatalf("request body not deterministic:\n%s\n%s", first, second)
	}

	var snap map[string]any
	_ = json.Unmarshal(first, &snap)
	inner, ok := snap["request"].(map[string]any)
	if !ok {
		t.Fatalf("snapshot missing request object: %s", first)
	}
	for _, key := range []string{"contents", "sessionId", "toolConfig", "generationConfig", "systemInstruction", "tools"} {
		if _, ok := inner[key]; !ok {
			t.Fatalf("snapshot missing request.%s: %s", key, first)
		}
	}
	if snap["project"] != "proj-1" || snap["model"] != "claude-sonnet-4-5" || snap["requestType"] != "agent" || snap["userAgent"] != "antigravity" {
		t.Fatalf("unexpected envelope: %s", first)
	}
	if inner["sessionId"] != generateStableSessionID(userText("hi"), opts.SessionScope) {
		t.Fatalf("sessionId not derived from scope: %v", inner["sessionId"])
	}
}

func TestTransformClaudeToGemini_ConcurrentSessionIsolation(t *testing.T) {
	const workers = 32
	ids := make([]string, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := &ClaudeRequest{
				Model:     "claude-sonnet-4-5",
				MaxTokens: 64,
				Messages:  []ClaudeMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}},
			}
			opts := DefaultTransformOptions()
			opts.SessionScope = "account:1|api_key:" + strconv.Itoa(i)
			body, err := TransformClaudeToGeminiWithOptions(req, "p", "claude-sonnet-4-5", opts)
			if err != nil {
				t.Errorf("transform: %v", err)
				return
			}
			var m struct {
				Request struct {
					SessionID string `json:"sessionId"`
				} `json:"request"`
			}
			_ = json.Unmarshal(body, &m)
			ids[i] = m.Request.SessionID
		}(i)
	}
	wg.Wait()
	seen := map[string]int{}
	for i, id := range ids {
		if prev, dup := seen[id]; dup {
			t.Fatalf("users %d and %d share sessionId %s for the same opener", prev, i, id)
		}
		seen[id] = i
	}
}
