package live

import (
	"net/http"
	"strings"
	"testing"
)

func TestRewriteQuicksilverAlphaVersionMapsV2ToV1(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "quicksilver=v2", want: "quicksilver=v1"},
		{in: "quicksilver=v1", want: "quicksilver=v1"},
		{in: "other=1, quicksilver=v2", want: "other=1, quicksilver=v1"},
		{in: "", want: "quicksilver=v1"},
		{in: "beta=1", want: "beta=1, quicksilver=v1"},
	}
	for _, tc := range tests {
		if got := rewriteQuicksilverAlphaVersion(tc.in, "v1"); got != tc.want {
			t.Errorf("rewriteQuicksilverAlphaVersion(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRewriteCallCreateOpenAIAlphaSetsV1AndKeepsAVASURL(t *testing.T) {
	headers := http.Header{}
	headers.Set("OpenAI-Alpha", "quicksilver=v2")
	rewriteCallCreateOpenAIAlpha(headers)
	if got := headers.Get("OpenAI-Alpha"); got != "quicksilver=v1" {
		t.Fatalf("OpenAI-Alpha = %q, want quicksilver=v1", got)
	}
	if !strings.Contains(upstreamCallURL, "architecture=avas") {
		t.Fatalf("upstreamCallURL = %q, want architecture=avas", upstreamCallURL)
	}
	if !strings.Contains(upstreamCallURL, "intent=quicksilver") {
		t.Fatalf("upstreamCallURL = %q, want intent=quicksilver", upstreamCallURL)
	}
}

func TestRewriteCallCreateOpenAIAlphaFillsMissingHeader(t *testing.T) {
	headers := http.Header{}
	rewriteCallCreateOpenAIAlpha(headers)
	if got := headers.Get("OpenAI-Alpha"); got != "quicksilver=v1" {
		t.Fatalf("OpenAI-Alpha = %q, want quicksilver=v1", got)
	}
}
