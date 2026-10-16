package app

// Outcome describes how a run handled one container.
type Outcome string

// The outcomes a FileResult can carry.
const (
	// OutcomePlanned marks a container a dry run would have converted.
	OutcomePlanned Outcome = "planned"
	// OutcomeConverted marks a container whose audio was written.
	OutcomeConverted Outcome = "converted"
	// OutcomeSkipped marks a container whose destination already existed and
	// was left alone.
	OutcomeSkipped Outcome = "skipped"
	// OutcomeFailed marks a container that could not be converted.
	OutcomeFailed Outcome = "failed"
)

// FileResult reports how one container was handled.
type FileResult struct {
	// Source is the container the result is about.
	Source string
	// Dest is where the audio was written. It is empty when nothing was
	// written, as in a dry run or in a failure before the destination was
	// known.
	Dest string
	// Outcome is what happened to the container.
	Outcome Outcome
	// Err is why the container failed, and nil otherwise.
	Err error
}
