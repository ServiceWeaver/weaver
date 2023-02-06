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

package main

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"math"

	"golang.org/x/image/draw"
	"github.com/ServiceWeaver/weaver"
)

type ImageScaler interface {
	Scale(_ context.Context, img []byte, maxWidth, maxHeight int) ([]byte, error)
}

// scaler scales an image.
type scaler struct {
	weaver.Implements[ImageScaler]
}

// Scale resizes img so it has at most the specified maximum width and height
// while preserving its aspect ratio.
func (s *scaler) Scale(_ context.Context, img []byte, maxWidth, maxHeight int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewBuffer(img))
	if err != nil {
		return nil, err
	}

	bounds := src.Bounds()
	w := bounds.Max.X - bounds.Min.X
	h := bounds.Max.Y - bounds.Min.Y
	if w <= maxWidth && h <= maxHeight {
		return img, nil
	}

	// Find the smallest scaling ratio across the two dimensions.
	sx := float64(maxWidth) / float64(w)
	sy := float64(maxHeight) / float64(h)
	ratio := math.Min(sx, sy)
	scaledWidth := int(ratio * float64(w))
	scaledHeight := int(ratio * float64(h))
	dst := image.NewRGBA(image.Rect(0, 0, scaledWidth, scaledHeight))

	draw.NearestNeighbor.Scale(dst, dst.Rect, src, src.Bounds(), draw.Over, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
