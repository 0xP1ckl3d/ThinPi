//go:build !windows

package launch

import (
	"strings"
	"testing"
)

func TestClassifyClientFailure(t *testing.T) {
	cases := map[string]string{
		"failed to open display: :0":                      "display",
		"ERRCONNECT_LOGON_FAILURE":                        "username or password",
		"ERRCONNECT_CONNECT_TRANSPORT_FAILED":             "host and port",
		"certificate mismatch for remote system":          "certificate changed",
		"freerdp command line parsing failed at argument": "configuration",
	}
	for output, want := range cases {
		if got := classifyClientFailure(output).Error(); !strings.Contains(strings.ToLower(got), want) {
			t.Fatalf("%q: got %q, want %q", output, got, want)
		}
	}
}
