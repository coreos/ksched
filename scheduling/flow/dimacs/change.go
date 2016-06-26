// Copyright 2016 The ksched Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// The dimacs_change.h class equivalent
// This is meant to be inherited by its children clases such as dimacs_change_arc/node.h
// So we implement this as an interface to get the effect of polymorphism in the child classes

package dimacs

type Change interface {
	// Comment gets the change's comment
	Comment() string
	// SetComment sets the change's comment
	SetComment(string)
	// GenerateChangeDescription generates a dimacs comment line descriptor for this change
	GenerateChangeDescription() string
	// GenerateChange generates dimacs line descriptor for this change
	GenerateChange() string
}

type commentChange struct{ comment string }

func (cc *commentChange) Comment() string           { return cc.comment }
func (cc *commentChange) SetComment(comment string) { cc.comment = comment }
func (cc *commentChange) GenerateChangeDescription() string {
	if cc.comment == "" {
		return ""
	}
	return "c " + cc.comment + "\n"
}
