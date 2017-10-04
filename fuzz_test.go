package flac

import (
	"strings"
	"testing"
)

func TestFuzzCrashers(t *testing.T) {

	var crashers = []string{
		"fLaC\x00000000000000000000000",
		"fLaC\x04000",
		"fLaC\x840000000",
	}

	for _, f := range crashers {
		Decode(strings.NewReader(f))
	}
}
