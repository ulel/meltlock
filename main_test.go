package main

import (
	"slices"
	"testing"
)

func TestLockerArgsDaemonizeByDefault(t *testing.T) {
	if got := lockerArgs(nil); !slices.Equal(got, lockArgs) {
		t.Fatalf("lockerArgs(nil) = %v, want lockArgs", got)
	}
}

func TestLockerArgsPassedThrough(t *testing.T) {
	got := lockerArgs([]string{"--color", "000000"})
	if !slices.Contains(got, "--daemonize") {
		t.Error("--daemonize dropped without --ready-fd")
	}
	if !slices.Equal(got[len(got)-2:], []string{"--color", "000000"}) {
		t.Errorf("extra options not last: %v", got)
	}
}

func TestLockerArgsSupervisedStaysInForeground(t *testing.T) {
	for _, extra := range [][]string{
		{"--ready-fd", "3"},
		{"--ready-fd=3"},
		{"-R", "3"},
		{"-R3"},
	} {
		got := lockerArgs(extra)
		if slices.Contains(got, "--daemonize") {
			t.Errorf("lockerArgs(%v) daemonizes", extra)
		}
		if !slices.Equal(got[len(got)-len(extra):], extra) {
			t.Errorf("lockerArgs(%v) lost the option: %v", extra, got)
		}
	}
}
