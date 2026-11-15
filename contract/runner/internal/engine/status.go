package engine

import "jitso-contract-runner/internal/spec"

// Status is the reported outcome of one case expansion (SPEC 8).
type Status string

const (
	Pass  Status = "PASS"
	Fail  Status = "FAIL"
	Xfail Status = "XFAIL"
	Xpass Status = "XPASS"
	Skip  Status = "SKIP"
)

// Classify maps the declared status and the outcome to a reported status. A runner error
// (the sandbox could not be built, a placeholder had no value at run time) is a FAIL even
// for an xfail case: it says nothing about the defect.
func Classify(declared string, held, runnerError bool) Status {
	switch {
	case runnerError:
		return Fail
	case declared == spec.StatusXfail && held:
		return Xpass
	case declared == spec.StatusXfail:
		return Xfail
	case held:
		return Pass
	}
	return Fail
}

// ExitCode is 2 when any case failed to load, else 1 when any case is FAIL or XPASS, else 0.
func ExitCode(statuses []Status, loadErrors int) int {
	if loadErrors > 0 {
		return 2
	}
	for _, s := range statuses {
		if s == Fail || s == Xpass {
			return 1
		}
	}
	return 0
}
