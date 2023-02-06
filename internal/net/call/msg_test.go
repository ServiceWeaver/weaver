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

package call

import (
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"testing"
	"time"
)

func TestConcurrentWrites(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	extraHdr := []byte{
		// 16 byte method fingerprint.
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		// 8 byte deadline.
		0, 0, 0, 0, 0, 0, 0, 0,
	}
	payload := []byte{
		// 5 byte payload.
		10, 20, 30, 40, 50,
	}
	fullPayload := append(extraHdr, payload...)

	var wlock sync.Mutex
	numWriters := 2
	numWrites := 100
	writer := func(id int) error {
		for i := 0; i < numWrites; i++ {
			flattenLimit := 0
			if rand.Int()%2 == 0 {
				flattenLimit = 9999999
			}
			if err := writeMessage(client, &wlock, requestMessage, uint64(id), extraHdr, payload, flattenLimit); err != nil {
				return err
			}
			id += numWriters
		}
		return nil
	}

	reader := func() error {
		for i := 0; i < numWriters*numWrites; i++ {
			mt, id, payload, err := readMessage(server)
			if err != nil {
				return err
			}
			if got, want := mt, requestMessage; got != want {
				return fmt.Errorf("bad messageType: got %d, want %d", got, want)
			}
			if min, max := 0, numWriters*numWrites; int(id) < min || int(id) > max {
				return fmt.Errorf("bad id: got %d, want in range [%d, %d]", id, min, max)
			}
			if got, want := payload, fullPayload; !bytes.Equal(got, want) {
				return fmt.Errorf("bad payload:\ngot %v\nwant %v", got, want)
			}
		}
		return nil
	}

	// Launch the goroutines.
	errs := make(chan error, numWriters+1)
	for i := 0; i < numWriters; i++ {
		i := i
		go func() { errs <- writer(i) }()
	}
	go func() { errs <- reader() }()

	// Wait for the goroutines to finish.
	timeout := time.NewTimer(1 * time.Second)
	for i := 0; i < numWriters; i++ {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

		case <-timeout.C:
			t.Fatal("test timed out")
		}
	}
}

func BenchmarkReadWrite(b *testing.B) {
	for _, network := range []string{"tcp"} {
		out, in := net.Pipe()
		for _, size := range []int{1, 100, 500, 1 << 10, 1 << 12, 1 << 14, 1 << 16} {
			for _, flatten := range []string{"Flatten", "NoFlatten"} {
				flatten := flatten
				name := fmt.Sprintf("%s/%s/%s", network, sizeString(size), flatten)
				b.Run(name, func(b *testing.B) {
					numIters := b.N
					payload := make([]byte, size)
					var mu sync.Mutex
					var extraHdr [1]byte
					done := make(chan bool)
					go func() {
						for n := 0; n < numIters; n++ {
							if _, _, _, err := readMessage(in); err != nil {
								panic(fmt.Sprint(err))
							}
						}
						done <- true
					}()
					for n := 0; n < numIters; n++ {
						if flatten == "Flatten" {
							if err := writeFlat(out, &mu, requestMessage, 0, extraHdr[:], payload); err != nil {
								b.Fatal(err)
							}
						} else {
							if err := writeChunked(out, &mu, requestMessage, 0, extraHdr[:], payload); err != nil {
								b.Fatal(err)
							}
						}
					}
					<-done
				})
			}
		}
	}
}

func sizeString(s int) string {
	if s >= 1048576 {
		return fmt.Sprintf("%gM", float64(s)/1048576)
	}
	if s >= 1024 {
		return fmt.Sprintf("%gK", float64(s)/1024)
	}
	return fmt.Sprint(s)
}
