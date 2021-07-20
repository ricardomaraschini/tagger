// Copyright 2020 The Tagger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package progbar

import (
	"context"

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
func New(ctx context.Context, desc string) *ProgressBar {
	return &ProgressBar{
		desc: desc,
		prog: mpb.NewWithContext(ctx, mpb.WithWidth(60)),
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

// Abort cancels the progress bar.
func (p *ProgressBar) Abort() {
	p.pbar.Abort(true)
}

// Wait awaits for the ProgressBar to finish drawing.
func (p *ProgressBar) Wait() {
	p.prog.Wait()
}
