package llm

import (
	"testing"
)

func TestIsThinkingUnsupported(t *testing.T) {
	for _, tc := range []struct{ errText string; want bool }{
		{"model does not support thinking", true},
		{"not support thinking mode", true},
		{"thinking is not supported", true},
		{"reasoning_effort is invalid", true},
		{"some other error", false},
		{"", false},
	} {
		if got := isThinkingUnsupported(tc.errText); got != tc.want {
			t.Errorf("isThinkingUnsupported(%q) = %v, want %v", tc.errText, got, tc.want)
		}
	}
}

func TestOpenAIChatEndpoint(t *testing.T) {
	for _, tc := range []struct{ base, want string }{
		{"https://api.example.com/v1", "https://api.example.com/v1/chat/completions"},
		{"https://api.example.com/v1/", "https://api.example.com/v1/chat/completions"},
		{"https://api.example.com/v1/chat/completions", "https://api.example.com/v1/chat/completions"},
		{"https://api.example.com", "https://api.example.com/v1/chat/completions"},
		{"http://localhost:8000", "http://localhost:8000/v1/chat/completions"},
	} {
		if got := openAIChatEndpoint(tc.base); got != tc.want {
			t.Errorf("openAIChatEndpoint(%q) = %q, want %q", tc.base, got, tc.want)
		}
	}
}

func TestParseOpenAIContent(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"standard", `{"choices":[{"message":{"content":"Hello"}}]}`, "Hello"},
		{"text field", `{"choices":[{"text":"Hello"}]}`, "Hello"},
		{"empty choices", `{"choices":[]}`, ""},
		{"invalid", "not json", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseOpenAIContent([]byte(tc.body)); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseOpenAIStreamChunk(t *testing.T) {
	for _, tc := range []struct{ name, data, wantContent, wantReasoning string }{
		{"content", `{"choices":[{"delta":{"content":"Hi"}}]}`, "Hi", ""},
		{"reasoning", `{"choices":[{"delta":{"reasoning_content":"think"}}]}`, "", "think"},
		{"text", `{"choices":[{"text":"Hi"}]}`, "Hi", ""},
		{"empty", `{"choices":[]}`, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, r := parseOpenAIStreamChunk([]byte(tc.data))
			if c != tc.wantContent || r != tc.wantReasoning {
				t.Errorf("content=%q reasoning=%q", c, r)
			}
		})
	}
}

func TestImageMIME(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"img.png", "image/png"}, {"photo.jpg", "image/jpeg"},
		{"photo.jpeg", "image/jpeg"}, {"anim.gif", "image/gif"},
		{"pic.webp", "image/webp"}, {"pic.bmp", "image/bmp"},
	} {
		if got := imageMIME(tc.path); got != tc.want {
			t.Errorf("imageMIME(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}