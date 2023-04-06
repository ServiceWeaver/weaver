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

package weaver

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/DataDog/hyperloglog"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/lightstep/varopt"
)

func approxEqual(a, b float64) bool {
	const float64EqualityThreshold = 1e-9
	return math.Abs(a-b) <= float64EqualityThreshold
}

// TODO(mwhittaker): Right now, load collection is slow. It grabs a mutex
// every time the load needs to be updated. Make this faster.

// loadCollector collects load for a Service Weaver component. As an example, imagine we
// have a load collector lc for a Service Weaver component that owns slices [0, 10) and
// [100, 200). We add the following load over the course of a second.
//
//   - lc.Add(0, 1)
//   - lc.Add(1, 1)
//   - lc.Add(2, 1)
//   - lc.Add(3, 1)
//   - lc.Add(100, 1)
//   - lc.Add(101, 1)
//
// The load collector will report a load of 4 requests per second on the slice
// [0, 10) and a load of 2 requests per second on the slice [100, 200).
type loadCollector struct {
	component string           // Service Weaver component
	addr      string           // dialable address found in assignments
	now       func() time.Time // time.Now usually, but injected fake in tests

	mu         sync.Mutex               // guards the following fields
	assignment *protos.Assignment       // latest assignment
	index      index                    // index on assignment
	start      time.Time                // start of load collection
	slices     map[uint64]*sliceSummary // keyed by start of slice
}

// sliceSummary contains a summary of the observed keys and load of a slice for
// a replica.
type sliceSummary struct {
	slice  slice                    // the slice
	load   float64                  // total load
	count  *hyperloglog.HyperLogLog // counts distinct elements
	sample *varopt.Varopt           // reservoir sample of keys
}

// newLoadCollector returns a new load collector. Note that load is collected
// with respect to an assignment, so load won't be collected until
// UpdateAssignment is called.
func newLoadCollector(component string, addr string) *loadCollector {
	return &loadCollector{
		component: component,
		addr:      addr,
		now:       func() time.Time { return time.Now() },
		start:     time.Now(),
		slices:    map[uint64]*sliceSummary{},
	}
}

// add adds load for the provided key.
func (lc *loadCollector) add(key uint64, v float64) error {
	if v != 1.0 {
		panic("load != 1.0 not yet implemented")
	}

	// Find the corresponding slice.
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if lc.assignment == nil {
		// Load is reported with respect to a given assignment. If we don't
		// have an assignment yet, then we don't record the load.
		return nil
	}
	slice, found := lc.index.find(key)
	if !found {
		// TODO(mwhittaker): It is currently possible to receive a request for
		// a key that is not in our current assignment. For example, a
		// different weavelet may have an older or newer version of the current
		// assignment and send us keys not in our current assignment. In the
		// future, we may want to catch these requests and discard them. For
		// now, we execute them.
		return nil
	}
	if !slice.replicaSet[lc.addr] {
		return nil
	}

	summary, found := lc.slices[slice.start]
	if !found {
		var err error
		summary, err = newSliceSummary(slice)
		if err != nil {
			return err
		}
		lc.slices[slice.start] = summary
	}

	// Update the load.
	summary.load += v

	// Update the count. Note that we compute a hash of our key before passing
	// it to the hyperloglog, even though the key is itself a hash. The reason
	// is that this slice represents only a small sliver of the total hash
	// space. To operate correctly, a hyperloglog assumes values are drawn
	// uniformly from the space of all uint32s, so if we feed the hyperloglog
	// values only from this slice, the count will be inaccurate.
	//
	// TODO(mwhittaker): Compute the hash outside of the lock?
	// TODO(mwhittaker): Use a different sketch?
	// TODO(mwhittaker): If the slice is small (< 1024), we can count the
	// number of distinct elements exactly. Don't use a hyperloglog here.
	// TODO(mwhittaker): Start with an exact count and only switch to a
	// hyperloglog if the number of unique elements gets too big?
	summary.count.Add(hyperloglog.Murmur64(key))

	// Update the sample. Note that Add takes in a key and a weight, but we are
	// recording unweighted samples, so we use a constant weight of 1.0 for
	// every key.
	if _, err := summary.sample.Add(key, 1.0); err != nil {
		return fmt.Errorf("cannot sample %d: %v", key, err)
	}
	return nil
}

// updateAssignment updates a load collector with the latest assignment. The
// load reported by a load collector is always scoped to a single assignment.
// A load report never spans more than one assignment. Thus, UpdateAssignment
// also clears the load collector's accumulated load.
func (lc *loadCollector) updateAssignment(assignment *protos.Assignment) {
	index := newIndex(assignment)
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.assignment = assignment
	lc.index = index
	lc.start = lc.now()
	lc.slices = map[uint64]*sliceSummary{}
}

// report returns a report of the collected load. If the load collector
// doesn't have any collected load---this is possible if the load collector
// doesn't have an assignment yet---then Report returns nil.
func (lc *loadCollector) report() *protos.LoadReport_ComponentLoad {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	if lc.assignment == nil {
		return nil
	}

	now := lc.now()
	delta := now.Sub(lc.start)
	report := &protos.LoadReport_ComponentLoad{
		Version: lc.assignment.GetVersion(),
	}
	for _, summary := range lc.slices {
		report.Load = append(report.Load,
			&protos.LoadReport_SliceLoad{
				Start:  summary.slice.start,
				End:    summary.slice.end,
				Load:   summary.load / delta.Seconds(),
				Splits: summary.splits(delta),
				Size:   summary.count.Count(),
			})
	}
	sort.Slice(report.Load, func(i, j int) bool {
		return report.Load[i].Start < report.Load[j].Start
	})
	return report
}

// reset resets the load collector. If you want to collect load over 5
// minute windows, for example, call Reset every five minutes.
func (lc *loadCollector) reset() {
	lc.mu.Lock()
	defer lc.mu.Unlock()
	lc.start = lc.now()
	lc.slices = map[uint64]*sliceSummary{}
}

// newSliceSummary returns a new sliceSummary for the provided slice with
// initially 0 load.
func newSliceSummary(slice slice) (*sliceSummary, error) {
	// Initialize the hyperloglog. A hyperloglog with n registers uses roughly
	// n bytes of memory. We choose n=1024 so that every hyperloglog takes
	// about a kilobyte of memory. Given that a weavelet should manage a
	// moderate number of slices and components, the total memory usage of all
	// hyperloglogs should be relatively small. New's documentation also
	// suggests that n be a power of 2.
	count, err := hyperloglog.New(1024)
	if err != nil {
		return nil, err
	}

	// Initialize the reservoir sample. A reservoir sample of size n stores at
	// most n keys, or roughly 8n bytes. As with the hyperloglogs, this should
	// lead to a modest memory usage.
	//
	// TODO(mwhittaker): Compute the expected errors in our estimates based on
	// the size of the sample.
	// TODO(mwhittaker): When we switch to range sharding, keys might be large
	// and 1000 keys might be too big.
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	sample := varopt.New(1000, r)

	return &sliceSummary{slice: slice, count: count, sample: sample}, nil
}

// splits splits the slice into subslices with roughly even load.
func (s *sliceSummary) splits(delta time.Duration) []*protos.LoadReport_SubsliceLoad {
	// Splits divides the slice into subslices of roughly even load. In the
	// normal case, Splits splits a slice into 20 subslices, each representing
	// 5% of the total load. If the number of samples is small, however, fewer
	// splits are used. Moreover, if adjacent splits are formed from a single
	// hot key, they are combined.

	// Materialize and sort the sample.
	k := s.sample.Size()
	xs := make([]uint64, k)
	for i := 0; i < k; i++ {
		x, _ := s.sample.Get(i)
		xs[i] = x.(uint64)
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })

	// Determine the number of splits. More splits is better, but if we don't
	// have many points in our sample, then using a large number of splits will
	// lead to inaccurate estimates.
	var n int
	switch {
	case k < 10:
		n = 1 // 100%
	case k < 50:
		n = 2 // 50%
	case k < 100:
		n = 4 // 25%
	case k < 250:
		n = 5 // 20%
	case k < 500:
		n = 10 // 10%
	default:
		n = 20 // 5%
	}

	// Adjust the first subslice so that it starts at our slice boundary.
	totalLoad := s.load / delta.Seconds()
	splits := subslices(totalLoad, xs, n)
	splits[0].Start = s.slice.start

	// Double check that the split loads sum to the total load.
	var sum float64
	for _, split := range splits {
		sum += split.Load
	}
	if !approxEqual(sum, totalLoad) {
		panic(fmt.Sprintf("bad sum of split loads: got %f, want %f", sum, totalLoad))
	}

	return splits
}

// subslices returns n splits of the provided points with roughly the same
// load. For example, given xs = []uint64{10, 20, 30, 40, 50, 60, 70, 80}, n =
// 4, and a load of 10.0, subslices will return the following four splits:
//
//   - {Start: 10, Load: 2.5} // [10, 30)
//   - {Start: 30, Load: 2.5} // [30, 50)
//   - {Start: 50, Load: 2.5} // [50, 70)
//   - {Start: 70, Load: 2.5} // [70, infinity)
//
// The returned splits are as even as possible on a best effort basis.
// subslices only guarantees that the returned splits are contiguous and
// sorted.
//
// REQUIRES xs is sorted in increasing order
// REQUIRES n > 0
func subslices(load float64, xs []uint64, n int) []*protos.LoadReport_SubsliceLoad {
	quantum := load / float64(n)
	ps := percentiles(xs, n)
	subslices := []*protos.LoadReport_SubsliceLoad{{Start: ps[0], Load: quantum}}
	for _, p := range ps[1:] {
		last := subslices[len(subslices)-1]
		if last.Start != p {
			subslices = append(subslices, &protos.LoadReport_SubsliceLoad{Start: p, Load: quantum})
		} else {
			// Hot keys may occupy multiple slices. We merge these slices
			// together.
			last.Load += quantum
		}
	}
	return subslices
}

// percentiles returns n equally spaced percentiles of the provided sorted set
// of points. For example, given xs = []uint64{10, 20, 30, 40, 50, 60, 70, 80}
// and n = 4, percentiles will return []uint64{10, 30, 50, 70} where
//
//   - 10 is the 0th percentile,
//   - 30 is the 25th percentile,
//   - 50 is the 50th percentile,
//   - 70 is the 75th percentile,
//
// REQUIRES xs is sorted in increasing order
// REQUIRES n > 0
func percentiles(xs []uint64, n int) []uint64 {
	ps := make([]uint64, n)
	for i := 0; i < n; i++ {
		ps[i] = xs[int(float64(i)/float64(n)*float64(len(xs)))]
	}
	return ps
}

// index is a read-only search index of a protos.Assignment, optimized to
// find the slice that contains a key.
type index []slice

// slice is the segment [start, end) of the key space, along with a set of
// assigned replicas.
type slice struct {
	start      uint64          // start of slice, inclusive
	end        uint64          // end of slice, exclusive
	replicas   []string        // replicas assigned to this slice
	replicaSet map[string]bool // replicas assigned to this slice
}

// newIndex returns a new index of the provided assignment.
func newIndex(proto *protos.Assignment) index {
	n := len(proto.Slices)
	slices := make([]slice, n)
	for i := 0; i < n; i++ {
		// Gather the set of replicas.
		replicas := proto.Slices[i].Replicas
		replicaSet := make(map[string]bool, len(replicas))
		for _, replica := range replicas {
			replicaSet[replica] = true
		}

		// Compute the end of the slice.
		var end uint64 = math.MaxUint64
		if i < n-1 {
			end = proto.Slices[i+1].Start
		}

		// Form the slice.
		slices[i] = slice{
			start:      proto.Slices[i].Start,
			end:        end,
			replicas:   replicas,
			replicaSet: replicaSet,
		}
	}
	return slices
}

// find finds the slice that contains the given key in O(log n) time where n is
// the number of slices in the assignment.
func (ind index) find(key uint64) (slice, bool) {
	i := sort.Search(len(ind), func(i int) bool {
		return key < ind[i].end
	})
	if i == len(ind) {
		return slice{}, false
	}
	return ind[i], true
}
