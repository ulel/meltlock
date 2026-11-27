package main

import (
	"os/exec"
	"slices"
	"testing"
	"time"
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

func TestReadyFD(t *testing.T) {
	for _, c := range []struct {
		extra []string
		fd    int
		ok    bool
	}{
		{nil, -1, false},
		{[]string{"--color", "000000"}, -1, false},
		{[]string{"--ready-fd", "3"}, 3, true},
		{[]string{"--ready-fd=4"}, 4, true},
		{[]string{"-R", "5"}, 5, true},
		{[]string{"-R6"}, 6, true},
		{[]string{"--ready-fd"}, -1, true},
		{[]string{"--ready-fd", "x"}, -1, true},
	} {
		fd, ok := readyFD(c.extra)
		if fd != c.fd || ok != c.ok {
			t.Errorf("readyFD(%v) = %d, %v; want %d, %v", c.extra, fd, ok, c.fd, c.ok)
		}
	}
}

func TestWaitGoneWaitsForTheProcess(t *testing.T) {
	cmd := exec.Command("sleep", "0.3")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go cmd.Wait()
	start := time.Now()
	waitGone([]locker{{cmd.Process.Pid, "sleep"}}, 10*time.Millisecond)
	if d := time.Since(start); d < 200*time.Millisecond {
		t.Errorf("returned after %v, before the process exited", d)
	}
}

func TestWaitGoneIgnoresReusedPID(t *testing.T) {
	cmd := exec.Command("sleep", "5")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	start := time.Now()
	waitGone([]locker{{cmd.Process.Pid, "swaylock"}}, 10*time.Millisecond)
	if d := time.Since(start); d > 100*time.Millisecond {
		t.Errorf("waited %v on a process that is not the locker", d)
	}
}
