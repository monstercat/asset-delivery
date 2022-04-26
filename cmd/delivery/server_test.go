package main

import "testing"

func TestTestHostWithPattern(t *testing.T) {
	host := "abcdef1235939023.some-host.run.app"
	pattern := "*.some-host.run.app"
	if !testHostWithPattern(pattern, host) {
		t.Error("Host should match pattern but it does not.")
	}
	host = "some-host.some-host.notrun.app"
	if testHostWithPattern(pattern, host) {
		t.Error("Host should not match pattern but it does.")
	}
}