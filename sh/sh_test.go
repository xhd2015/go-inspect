package sh

import (
	"testing"
	"time"
)

// go test -run TestTimeout -v ./sh
func TestTimeout(t *testing.T) {
	// timeout options with normal output
	stdout, _, err := RunBashWithOpts([]string{
		"sleep 1; echo ok",
	}, RunBashOptions{
		NeedStdOut: true,
		Timeout:    2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stdout != "ok" {
		t.Fatalf("expect %s = %+v, actual:%+v", `stdout`, "ok", stdout)
	}

	// timeout happened
	_, _, err = RunBashWithOpts([]string{
		"sleep 2; echo ok",
	}, RunBashOptions{
		NeedStdOut: true,
		Timeout:    1 * time.Second,
	})
	if err == nil {
		t.Fatalf("expect timeout after 1s, actual not timeout")
	}
}
