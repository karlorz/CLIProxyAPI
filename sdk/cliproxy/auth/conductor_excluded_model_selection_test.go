package auth

import "testing"

func TestAuthExcludesModel(t *testing.T) {
	tests := map[string]struct {
		excluded string
		model    string
		want     bool
	}{
		"exact case insensitive": {excluded: "GPT-LIVE-1-CODEX", model: "gpt-live-1-codex", want: true},
		"wildcard":               {excluded: "gpt-live-*", model: "gpt-live-1-codex", want: true},
		"comma separated":        {excluded: "gpt-5.6-sol, gpt-live-1-codex", model: "gpt-live-1-codex", want: true},
		"different model":        {excluded: "gpt-5.6-sol", model: "gpt-live-1-codex", want: false},
		"blank pattern":          {excluded: "", model: "gpt-live-1-codex", want: false},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			credential := &Auth{Attributes: map[string]string{"excluded_models": test.excluded}}
			if got := authExcludesModel(credential, test.model); got != test.want {
				t.Fatalf("authExcludesModel() = %t, want %t", got, test.want)
			}
		})
	}
}
