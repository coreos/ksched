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

type DimacsChange interface {
	// Every struct implementing this interface must have this field
	// comment string

	// Named comment() in original
	GetComment() string
	SetComment() string
	// This had an implementation in the parent class
	// So it should be implemented as is given in the original file
	GenerateChangeDescription() string
	GenerateChange() string
}
