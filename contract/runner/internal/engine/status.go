package engine

// Status is the reported outcome of one case expansion (SPEC 8).
type Status string

const (
	Pass Status = "PASS"
	Fail Status = "FAIL"
	Skip Status = "SKIP"
)

// Classify maps the outcome to a reported status. A runner error (the sandbox could not be
// built, a placeholder had no value at run time) is a FAIL.
func Classify(held, runnerError bool) Status {
	if held && !runnerError {
		return Pass
	}
	return Fail
}

// ExitCode is 2 when any case failed to load, else 1 when any case is FAIL, else 0.
func ExitCode(statuses []Status, loadErrors int) int {
	if loadErrors > 0 {
		return 2
	}
	for _, s := range statuses {
		if s == Fail {
			return 1
		}
	}
	return 0
}
