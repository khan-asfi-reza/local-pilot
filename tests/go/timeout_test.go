package systemtest

import "time"

// timeoutAfterSeconds is a readable deadline channel for tests that must not
// hang the suite when a scheduler or stream goes wrong.
func timeoutAfterSeconds(n int) <-chan time.Time {
	return time.After(time.Duration(n) * time.Second)
}
