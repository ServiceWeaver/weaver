// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package colors contains color-related utilities.
package colors

import (
	"fmt"
	"os"

	"golang.org/x/term"
)

// Code represents an ANSI escape code for colors and formatting.
type Code string

const (
	Reset     Code = "\x1b[0m" // The ANSI escape code that resets formatting.
	Bold      Code = "\x1b[1m" // The ANSI escape code for bold text.
	Underline Code = "\x1b[4m" // The ANSI escape code for underlining.
)

// visible contains the indices of the 256 colors [1] that are easy to read
// on both light and dark backgrounds. Some colors---like black, white,
// dark blues, light yellows---can sometimes be hard to read.
//
// TODO(mwhittaker): Trim this list further. I removed the obviously hard
// to read colors, but there are still some that are a little hard to read.
// TODO(mwhittaker): Consider using RGB color codes instead.
//
// [1]: https://en.wikipedia.org/wiki/ANSI_escape_code#8-bit
var visible = []byte{
	1, 2, 3, 5, 6, 9, 10, 11, 12, 13,
	14, 22, 23, 24, 25, 26, 27, 28, 29, 30,
	31, 32, 33, 34, 35, 36, 37, 38, 39, 40,
	41, 42, 43, 44, 45, 46, 47, 48, 49, 50,
	51, 58, 62, 63, 64, 65, 66, 67, 68, 69,
	70, 71, 72, 73, 74, 75, 76, 77, 78, 79,
	80, 81, 82, 88, 89, 90, 91, 92, 93, 94,
	95, 96, 97, 98, 99, 100, 101, 103, 104,
	105, 106, 107, 110, 111, 112, 113, 114, 115, 116,
	117, 124, 125, 126, 127, 128, 129, 130, 131, 132,
	133, 134, 135, 136, 137, 138, 139, 140, 141, 142,
	143, 148, 160, 161, 162, 163, 164, 165, 166, 167,
	168, 169, 170, 171, 172, 173, 174, 175, 176, 177,
	178, 179, 180, 181, 182, 183, 184, 196, 197, 198,
	199, 200, 201, 202, 203, 204, 205, 206, 207, 208,
	209, 210, 211, 212, 213, 214, 215, 216, 217, 218,
	219,
}

// Enabled returns whether it is ok to write colorized output to stdout or
// stderr. It's true only if all the following conditions are met:
//
//   - The environment variable NO_COLOR is not set.
//   - The environment variable TERM is not equal to "dumb".
//   - stdout and stderr are both terminals.
func Enabled() bool {
	// This logic was taken from https://github.com/fatih/color/blob/main/color.go.
	_, exists := os.LookupEnv("NO_COLOR")
	return !exists &&
		os.Getenv("TERM") != "dumb" &&
		term.IsTerminal(int(os.Stdout.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

// Color256 returns the ANSI escape code for the provided 256 (a.k.a. 8-bit)
// foreground color. See [1] for details and a depiction of the color space.
//
// [1]: https://en.wikipedia.org/wiki/ANSI_escape_code#8-bit
func Color256(i byte) Code {
	return Code(fmt.Sprintf("\x1b[38;5;%dm", i))
}

// ColorHash returns a 256 color based on the hash of the provided string.
func ColorHash(s string) Code {
	// TODO(mwhittaker): Right now, we use a dumb "hash" (i.e. the sum of the
	// bytes in the string). Replace this with a better hash if needed.
	var hash byte
	for i := 0; i < len(s); i++ {
		hash += s[i]
	}
	return Color256(visible[int(hash)%len(visible)])
}
