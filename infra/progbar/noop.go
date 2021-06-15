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
