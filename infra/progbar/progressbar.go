package progbar

import (
	"fmt"
	"sync"

	"github.com/schollz/progressbar/v3"
)

// ProgressBar handles a progress bar drawing on a terminal.
type ProgressBar struct {
	mtx  sync.Mutex
	pbar *progressbar.ProgressBar
	desc string
}

// New returns a new ProgressBar using the provided description as the bar label.
func New(desc string) *ProgressBar {
	return &ProgressBar{
		desc: desc,
	}
}

// SetCurrent sets the current value of the ProgressBar.
func (p *ProgressBar) SetCurrent(cur int64) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if p.pbar == nil {
		return
	}
	p.pbar.Set64(cur)
}

// SetMax sets the max value for the ProgressBar.
func (p *ProgressBar) SetMax(max int64) {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	if p.pbar != nil {
		return
	}
	p.pbar = progressbar.NewOptions64(
		max,
		progressbar.OptionSetPredictTime(false),
		progressbar.OptionSetWidth(50),
		progressbar.OptionSetDescription(p.desc),
		progressbar.OptionShowBytes(true),
		progressbar.OptionSetTheme(
			progressbar.Theme{
				Saucer:        "▮",
				SaucerHead:    "▮",
				SaucerPadding: "▯",
				BarStart:      " ",
				BarEnd:        " ",
			},
		),
	)
}

// Wait awaits for the ProgressBar to finish drawing.
func (p *ProgressBar) Wait() {
	p.mtx.Lock()
	defer p.mtx.Unlock()

	p.pbar.Close()
	fmt.Println("")
}
