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

// Package benchmarks provides various benchmarks to evaluate the performance of
// the Service Weaver ecosystem.

package benchmarks

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"testing"

	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/weavertest"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"
)

const (
	maxPings = 10 // Maximum number of chained Pings
	charsReq = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
)

// codec is the interface for something that can encode and decode values of
// type payloadC.
type codec interface {
	Encode(*payloadC) []byte
	Decode([]byte) *payloadC
}

// weaverCodec uses Service Weaver's serialization and deserialization code.
type weaverCodec struct {
	presizeBuffer bool             // presize the buffer? (Service Weaver does this)
	enc           *codegen.Encoder // reusable encoder, if not nil
}

// newWeaverCodec returns a new weaverCodec. If presizeBuffer is true, the
// serialization buffer is presized to fit the serialization (Service Weaver does this
// by default). If reuseBuffer is true, then the encoder reuses the same buffer
// for every serialization.
func newWeaverCodec(presizeBuffer, reuseBuffer bool) *weaverCodec {
	d := weaverCodec{presizeBuffer: presizeBuffer}
	if reuseBuffer {
		d.enc = codegen.NewEncoder()
	}
	return &d
}

func (d *weaverCodec) Encode(p *payloadC) []byte {
	if d.enc != nil {
		// Reset the buffer, but reuse the existing allocated memory.
		d.enc.Reset(0)
		p.WeaverMarshal(d.enc)
		return d.enc.Data()
	}

	// Use a new encoder (which internally allocates a new serialization
	// buffer).
	enc := codegen.NewEncoder()
	if d.presizeBuffer {
		enc.Reset(serviceweaver_size_payloadC_7e82696e(p))
	}
	p.WeaverMarshal(enc)
	return enc.Data()
}

func (d *weaverCodec) Decode(b []byte) *payloadC {
	var p payloadC
	dec := codegen.NewDecoder(b)
	p.WeaverUnmarshal(dec)
	return &p
}

// jsonCodec uses json serialization and deserialization.
type jsonCodec struct {
	enc *json.Encoder // reusable encoder, if not nil
	buf bytes.Buffer  // buffer used by enc
}

// newJsonCodec returns a new jsonCodec. If reuseBuffer is true, then the
// encoder reuses the same buffer for every serialization.
func newJsonCodec(reuseBuffer bool) *jsonCodec {
	j := jsonCodec{}
	if reuseBuffer {
		j.enc = json.NewEncoder(&j.buf)
	}
	return &j
}

func (j *jsonCodec) Encode(p *payloadC) []byte {
	if j.enc != nil {
		// Reset the buffer, but reuse the existing allocated memory.
		j.buf.Reset()
		if err := j.enc.Encode(p); err != nil {
			panic(err)
		}
		return j.buf.Bytes()
	}

	// Use a fresh encoder.
	bytes, err := json.Marshal(p)
	if err != nil {
		panic(err)
	}
	return bytes
}

func (j *jsonCodec) Decode(b []byte) *payloadC {
	var p payloadC
	if err := json.Unmarshal(b, &p); err != nil {
		panic(err)
	}
	return &p
}

// freshGobCodec uses gob serialization and deserialization, with a fresh
// gob.Encoder and gob.Decoder every time. This is *not* how gob is designed to
// be used. It exists only to show the performance difference of gob when it's
// used correctly vs when it's used incorrectly.
type freshGobCodec struct{}

func (freshGobCodec) Encode(p *payloadC) []byte {
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(p); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func (freshGobCodec) Decode(b []byte) *payloadC {
	buf := bytes.NewBuffer(b)
	dec := gob.NewDecoder(buf)
	var p payloadC
	if err := dec.Decode(&p); err != nil {
		panic(err)
	}
	return &p
}

// gobCodec uses gob serialization and deserialization.
type gobCodec struct {
	reuseBuffer bool         // reuse serialization buffer?
	enc         *gob.Encoder // encoder
	encBuf      bytes.Buffer // buffer used by enc
	dec         *gob.Decoder // decoder
	decBuf      bytes.Buffer // buffer used by dec
}

// newGobCodec returns a new gobCodec. If reuseBuffer is true, then the
// encoder reuses the same buffer for every serialization.
func newGobCodec(reuseBuffer bool) *gobCodec {
	g := gobCodec{reuseBuffer: reuseBuffer}
	g.enc = gob.NewEncoder(&g.encBuf)
	g.dec = gob.NewDecoder(&g.decBuf)

	// We need to encode and decode a message to make sure the encoder and
	// decoder both understand the type payloadC.
	var p payloadC
	if err := g.enc.Encode(p); err != nil {
		panic(err)
	}
	if _, err := io.Copy(&g.decBuf, &g.encBuf); err != nil {
		panic(err)
	}
	if err := g.dec.Decode(&p); err != nil {
		panic(err)
	}
	g.encBuf.Reset()
	g.decBuf.Reset()

	return &g
}

func (g *gobCodec) Encode(p *payloadC) []byte {
	if g.reuseBuffer {
		g.encBuf.Reset()
		if err := g.enc.Encode(p); err != nil {
			panic(err)
		}
		return g.encBuf.Bytes()
	}

	if err := g.enc.Encode(p); err != nil {
		panic(err)
	}
	// Read the bytes. Note that if we only call g.encBuf.Bytes(), the returned
	// bytes will include every serialization, not just the most recent.
	bytes, err := io.ReadAll(&g.encBuf)
	if err != nil {
		panic(err)
	}
	return bytes
}

func (g *gobCodec) Decode(b []byte) *payloadC {
	if _, err := g.decBuf.Write(b); err != nil {
		panic(err)
	}
	var p payloadC
	if err := g.dec.Decode(&p); err != nil {
		panic(err)
	}
	return &p
}

// protoCodec uses protobuf serialization and deserialization. Note that there
// is overhead in converting between payloadC and PayloadCProto, but this is an
// overhead we would have to incur if we used protobuf serialization in Service Weaver
// since the user doesn't operate on protobuf types.
type protoCodec struct{}

func (protoCodec) Encode(p *payloadC) []byte {
	bytes, err := proto.Marshal(p.ToProto())
	if err != nil {
		panic(err)
	}
	return bytes
}

func (protoCodec) Decode(b []byte) *payloadC {
	var p PayloadCProto
	if err := proto.Unmarshal(b, &p); err != nil {
		panic(err)
	}
	var payload payloadC
	payload.FromProto(&p)
	return &payload
}

// BenchmarkEncDec builds a complex payload, and invokes the:
//
//	(1) Service Weaver encoding
//	(2) JSON encoding
//	(3) gob encoding
func BenchmarkEncDec(b *testing.B) {
	payloads := genWorkload(1000)
	averageSize := func(c codec) int64 {
		totalSize := 0
		for _, p := range payloads {
			totalSize += len(c.Encode(p))
		}
		return int64(totalSize / len(payloads))
	}

	for _, bench := range []struct {
		name  string
		codec codec
	}{
		{"ServiceWeaver", newWeaverCodec(true, false)},
		{"ServiceWeaverNoPresize", newWeaverCodec(false, false)},
		{"ServiceWeaverReuseBuffer", newWeaverCodec(true, true)},
		{"Json", newJsonCodec(false)},
		{"JsonReuseBuffer", newJsonCodec(true)},
		{"FreshGob", freshGobCodec{}},
		{"Gob", newGobCodec(false)},
		{"GobReuseBuffer", newGobCodec(true)},
		{"Proto", protoCodec{}},
	} {
		// Encode only.
		b.Run(fmt.Sprintf("%s/Encode", bench.name), func(b *testing.B) {
			b.SetBytes(averageSize(bench.codec))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				bench.codec.Encode(payloads[rand.Intn(len(payloads))])
			}
		})

		// Decode only.
		encoded := bench.codec.Encode(payloads[0])
		b.Run(fmt.Sprintf("%s/Decode", bench.name), func(b *testing.B) {
			b.SetBytes(averageSize(bench.codec))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				bench.codec.Decode(encoded)
			}
		})

		// Encode and decode.
		b.Run(fmt.Sprintf("%s/EncodeDecode", bench.name), func(b *testing.B) {
			b.SetBytes(averageSize(bench.codec))
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				encoded := bench.codec.Encode(payloads[rand.Intn(len(payloads))])
				bench.codec.Decode(encoded)
			}
		})
	}
}

// BenchmarkPing tests the performance of sending a payload of a given size
// through an N-deep chain of components.
//
// TODO(rgrandl): we need to kill the corresponding processes once the benchmark
// finishes.
func BenchmarkPing(b *testing.B) {
	// Special size to use when we want a complex payload.
	const complex = -1

	benchmarks := []struct {
		numChainedComponents int // number of chained components to serve a given request
		componentSize        int // complex, or square root of approximate byte size
	}{
		{numChainedComponents: 1, componentSize: complex},
		{numChainedComponents: 10, componentSize: complex},
		{numChainedComponents: 1, componentSize: 10},
		{numChainedComponents: 1, componentSize: 100},
		{numChainedComponents: 1, componentSize: 1000},
		{numChainedComponents: 10, componentSize: 100},
	}

	ctx := context.Background()
	for _, bm := range benchmarks {
		b.ResetTimer()
		size := fmt.Sprintf("%07d", bm.componentSize*bm.componentSize)
		if bm.componentSize == complex {
			size = "complex"
		}
		name := fmt.Sprintf("chain=%02d,size=%s", bm.numChainedComponents, size)
		b.Run(name, func(b *testing.B) {
			weavertest.Local.Bench(b, func(b *testing.B, pObj Ping1) {
				if bm.componentSize == complex {
					payload := genWorkload(1)[0]
					for i := 0; i < b.N; i++ {
						_, err := pObj.PingC(ctx, *payload, bm.numChainedComponents)
						if err != nil {
							b.Fatal(err)
						}
					}
				} else {
					payload := buildPayloadS(bm.componentSize)
					for i := 0; i < b.N; i++ {
						_, err := pObj.PingS(ctx, payload, bm.numChainedComponents)
						if err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		})
	}
}

// TestBenchmark is a test to ensure that BenchmarkPing works correctly.
func TestBenchmark(t *testing.T) {
	// Test plan: Send a ping request from Component1 to Component10. Verify that
	// the response is the same as the request when we send both simple and complex payloads.
	weavertest.Local.Test(t, func(t *testing.T, ping Ping1) {
		ctx := context.Background()
		depth := 10

		// Test 1
		reqPayload := buildPayloadS(100)
		respPayload, err := ping.(Ping).PingS(ctx, reqPayload, depth)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(respPayload.Values, reqPayload.Values); diff != "" {
			t.Fatalf("list: (-want,+got):\n%s\n", diff)
		}

		// Test 2
		reqPayloadC := genWorkload(1)[0]
		respPayloadC, errC := ping.(Ping).PingC(ctx, *reqPayloadC, depth)
		if errC != nil {
			t.Fatal(errC)
		}
		if diff := cmp.Diff(respPayloadC.E, reqPayloadC.E); diff != "" {
			t.Fatalf("list: (-want,+got):\n%s\n", diff)
		}
	})
}

// genWorkload generates a slice of length n of payloadC components that emulates
// a production payload.
func genWorkload(n int) []*payloadC {
	a := make([]*payloadC, 0, n)
	for i := 0; i < n; i++ {
		p := payloadC{
			A: rand.Float64(),
			B: randStringBytes(100),
			C: rand.Int63(),
			E: randStringBytes(2000),
			F: rand.Int63(),
			H: randStringBytes(2000),
			I: rand.Int63(),
			J: rand.Float32(),
			K: randStringBytes(200),
		}
		b := make([]bool, 5)
		for i := 0; i < 5; i++ {
			if rand.Intn(1000) <= 500 {
				b[i] = false
			} else {
				b[i] = true
			}
		}
		vi := make([]int64, 15)
		for i := 0; i < 15; i++ {
			vi[i] = rand.Int63()
		}
		p.G.A = b
		p.D.B = vi
		p.D.A.A.B = rand.Int63()
		p.D.A.A.C = rand.Int63()
		p.D.A.A.A.A = rand.Int63()
		p.D.A.A.A.C = rand.Int63()
		p.D.A.A.A.C = rand.Int63()
		p.D.A.A.A.B.A = rand.Int63()
		p.D.A.A.A.B.B = rand.Int63()

		a = append(a, &p)
	}
	return a
}

// buildPayloadS returns a payload that has:
//   - n string fields;
//   - each field contains n characters.
func buildPayloadS(n int) payloadS {
	vals := []string{}
	for i := 0; i < n; i++ {
		vals = append(vals, randStringBytes(n))
	}
	return payloadS{Values: vals}
}

func randStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = charsReq[rand.Intn(len(charsReq))]
	}
	return string(b)
}
