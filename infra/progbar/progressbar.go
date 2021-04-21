package progbar

import (
	"github.com/vbauerster/mpb/v6"
	"github.com/vbauerster/mpb/v6/decor"
)

// ProgressBar handles a progress bar drawing on a terminal.
type ProgressBar struct {
	prog *mpb.Progress
	pbar *mpb.Bar
	desc string
}

// New returns a new ProgressBar using the provided description as the bar label.
func New(desc string) *ProgressBar {
	return &ProgressBar{
		desc: desc,
		prog: mpb.New(mpb.WithWidth(60)),
	}
}

// SetCurrent sets the current value of the ProgressBar.
func (p *ProgressBar) SetCurrent(cur int64) {
	if p.pbar == nil {
		return
	}
	p.pbar.SetCurrent(cur)
}

// SetMax sets the max value for the ProgressBar.
func (p *ProgressBar) SetMax(max int64) {
	if p.pbar != nil {
		return
	}

	p.pbar = p.prog.Add(
		max,
		mpb.NewBarFiller(" ▮▮▯ "),
		mpb.PrependDecorators(decor.Name(p.desc)),
		mpb.AppendDecorators(decor.CountersKiloByte("%d %d")),
	)
}

// Wait awaits for the ProgressBar to finish drawing.
func (p *ProgressBar) Wait() {
	p.prog.Wait()
}
