package langdetect

import "testing"

func TestDetectISO6391(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "blank", text: " ", want: ""},
		{name: "too short", text: "hello", want: ""},
		{name: "english", text: "This is a simple English sentence with enough letters.", want: "en"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := DetectISO6391(tt.text); got != tt.want {
				t.Fatalf("DetectISO6391(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}
