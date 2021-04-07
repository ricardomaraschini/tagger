package progbar

// NoOp is a progress bar that does not draw anything.
type NoOp struct{}

// NewNoOp returns a new NoOp progress bar. NoOp progress bar does nothing.
func NewNoOp() *NoOp {
	return &NoOp{}
}

// SetCurrent sets the current value.
func (n *NoOp) SetCurrent(cur int64) {
}

// SetMax sets the max value.
func (n *NoOp) SetMax(max int64) {
}
