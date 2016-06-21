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

// Common unit conversion constants.

package units

const (
	// Capacity
	BytesToMb = 1024 * 1024
	BytesToGb = 1024 * 1024 * 1024
	KbToBytes = 1024
	KbToMb    = 1024
	KbToGb    = 1024 * 1024
	MbToBytes = 1024 * 1024

	// Bandwidth
	BytesToMbits = 8 * 1000 * 1000
	BytesToGbits = 8 * 1000 * 1000 * 1000
)
