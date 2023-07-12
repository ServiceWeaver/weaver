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

package metrics

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/ServiceWeaver/weaver/runtime/codegen"
	"github.com/ServiceWeaver/weaver/runtime/logging"
	"github.com/ServiceWeaver/weaver/runtime/metrics"
)

// TODO(rgrandl): Right now we aggregate local and remote metrics. Show them separately.

// StatsProcessor keeps track of various statistics for a given app deployment.
type StatsProcessor struct {
	mu    sync.Mutex
	start time.Time                          // time when the first set of stats was ever computed
	stats map[string]map[string]*statsMethod // per component, per method
}

// statsMethod contains stats maintained by the statsProcessor for a given method.
type statsMethod struct {
	name string // Name of the method

	// Slice of stats buckets, where each bucket contains stats for a given minute.
	// Note that we keep at most 60 buckets at any time, i.e., the stats processor
	// hold stats information only for the last one hour.
	//
	// TODO(rgrandl): Consider keeping a list of buckets at the top-level with an
	// entire snapshot stored in each bucket instead.
	buckets []*statsBucket
}

// statsBucket contains the stats recorded within a bucket. Note that each bucket
// contains stats for a given method (aggregated across all the replicas), for a 1-minute time interval.
type statsBucket struct {
	time          time.Time // timestamp at which these stats were computed
	calls         float64
	kbRecvd       float64
	kbSent        float64
	latencyMs     float64
	latencyCounts float64
}

func NewStatsProcessor() *StatsProcessor {
	return &StatsProcessor{stats: map[string]map[string]*statsMethod{}}
}

func (b *statsBucket) diff(o *statsBucket) *statsBucket {
	if o == nil {
		return b
	}
	return &statsBucket{
		calls:         b.calls - o.calls,
		kbRecvd:       b.kbRecvd - o.kbRecvd,
		kbSent:        b.kbSent - o.kbSent,
		latencyMs:     b.latencyMs - o.latencyMs,
		latencyCounts: b.latencyCounts - o.latencyCounts,
	}
}

func (b *statsBucket) add(o *statsBucket) {
	b.calls += o.calls
	b.kbRecvd += o.kbRecvd
	b.kbSent += o.kbSent
	b.latencyMs += o.latencyMs
	b.latencyCounts += o.latencyCounts
}

// AppStatuszInfo contains per app information to be displayed on the /statusz page.
type AppStatuszInfo struct {
	App        string
	Deployment string
	Age        string
	TraceFile  string
	Listeners  map[string][]string
	Config     map[string]string
	Components []ComponentStatuszInfo
}

// ComponentStatuszInfo contains per component information to be displayed on the /statusz page.
type ComponentStatuszInfo struct {
	Name        string              // Name of the component
	Replication int                 // Number of replicas
	Stats       []methodStatuszInfo // Per method status
}

// methodStatuszInfo contains per method information to be displayed on the /statusz page.
type methodStatuszInfo struct {
	Name   string // Name of the method
	Minute methodStats
	Hour   methodStats
	Total  methodStats
}

// methodStats contains a list of stats to be displayed on the /statusz page for a method.
type methodStats struct {
	NumCalls     float64
	AvgLatencyMs float64
	RecvKBPerSec float64
	SentKBPerSec float64
}

// CollectMetrics enables the stats processor to update the tracked stats based
// on a new set of metrics provided by snapshotFn.
func (s *StatsProcessor) CollectMetrics(ctx context.Context, snapshotFn func() []*metrics.MetricSnapshot) error {
	tickerCollectMetrics := time.NewTicker(time.Minute)
	defer tickerCollectMetrics.Stop()
	for {
		select {
		case <-tickerCollectMetrics.C:
			snapshot := snapshotFn()
			s.getSnapshot(snapshot)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// getSnapshot updates the tracked stats with a new set of stats, based on a list
// of metric snapshots.
func (s *StatsProcessor) getSnapshot(snapshot []*metrics.MetricSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.start.IsZero() {
		s.start = time.Now()
	}

	// Compute a new set of stats, keyed by component and method.
	newStats := map[string]map[string]*statsBucket{}
	for _, m := range snapshot {
		if !strings.HasPrefix(m.Name, "serviceweaver_") { // Ignore non-generated Service Weaver metrics
			continue
		}

		// Extract the component and the method name from the labels.
		var comp, method string
		for k, v := range m.Labels {
			if k == "component" {
				comp = logging.ShortenComponent(v)
			} else if k == "method" {
				method = v
			}
		}
		if comp == "" || method == "" {
			// Ignore any Service Weaver generated metric that doesn't have a
			// component and a method label set. E.g., http metrics.
			continue
		}

		if newStats[comp] == nil {
			newStats[comp] = map[string]*statsBucket{}
		}
		if newStats[comp][method] == nil {
			newStats[comp][method] = &statsBucket{time: time.Now()}
		}
		bucket := newStats[comp][method]

		// Aggregate stats within a bucket, based on metric values from different
		// replicas for the method.
		switch m.Name {
		case codegen.MethodCountsName:
			bucket.calls += m.Value
		case codegen.MethodBytesReplyName:
			bucket.kbSent += m.Value / 1024 // B to KB
		case codegen.MethodBytesRequestName:
			bucket.kbRecvd += m.Value / 1024 // B to KB
		case codegen.MethodLatenciesName:
			bucket.latencyMs += m.Value / 1000 // µs to ms

			var count uint64
			for idx := range m.Bounds {
				count += m.Counts[idx]
			}
			bucket.latencyCounts += float64(count)
		}
	}

	// Add the new stats buckets.
	for comp, compStats := range newStats {
		if s.stats[comp] == nil {
			s.stats[comp] = map[string]*statsMethod{}
		}
		for method, mstats := range compStats {
			if s.stats[comp][method] == nil {
				s.stats[comp][method] = &statsMethod{name: method}
			}
			sm := s.stats[comp][method]
			if len(sm.buckets) >= 60 { // Make sure we don't record more than 60 buckets.
				sm.buckets = sm.buckets[1:]
			}
			sm.buckets = append(s.stats[comp][method].buckets, mstats)
		}
	}
}

// GetStatsStatusz returns the latest stats that should be rendered on the /statusz page.
func (s *StatsProcessor) GetStatsStatusz() map[string][]methodStatuszInfo {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := map[string][]methodStatuszInfo{}
	for comp, compStats := range s.stats {
		for _, mstats := range compStats {
			result[comp] = append(result[comp], mstats.computeStatsStatusz(s.start))
		}
	}
	return result
}

// computeStatsStatusz computes the latest stats to be displayed on the /statusz page
// for a given method.
func (s *statsMethod) computeStatsStatusz(startTime time.Time) methodStatuszInfo {
	result := methodStatuszInfo{Name: s.name}
	if len(s.buckets) == 0 { // Nothing to display
		return result
	}

	totalTimeSec := time.Since(startTime).Seconds()
	lastBucket := s.buckets[len(s.buckets)-1]

	// Compute the overall stats.
	result.Total = methodStats{NumCalls: lastBucket.calls}
	if totalTimeSec > 0 {
		result.Total.SentKBPerSec = lastBucket.kbSent / totalTimeSec
		result.Total.RecvKBPerSec = lastBucket.kbRecvd / totalTimeSec
	}
	if lastBucket.latencyCounts > 0 {
		result.Total.AvgLatencyMs = lastBucket.latencyMs / lastBucket.latencyCounts
	}

	// Compute the last minute stats, i.e., the diff between the last bucket and
	// the previous one (if any). If there is a single bucket, it should reflect
	// the stats computed in the last 1 minute.
	durationSec := 60.0
	var prevBucket *statsBucket
	if len(s.buckets) > 1 {
		prevBucket = s.buckets[len(s.buckets)-2]
		durationSec = lastBucket.time.Sub(prevBucket.time).Seconds()
	}
	diffBucket := lastBucket.diff(prevBucket)
	result.Minute = methodStats{
		NumCalls:     diffBucket.calls,
		SentKBPerSec: diffBucket.kbSent / durationSec,
		RecvKBPerSec: diffBucket.kbRecvd / durationSec,
	}
	if diffBucket.latencyCounts > 0 {
		result.Minute.AvgLatencyMs = diffBucket.latencyMs / diffBucket.latencyCounts
	}

	// Compute the last hour stats.
	aggBucket := &statsBucket{
		calls:         s.buckets[0].calls,
		kbRecvd:       s.buckets[0].kbRecvd,
		kbSent:        s.buckets[0].kbSent,
		latencyMs:     s.buckets[0].latencyMs,
		latencyCounts: s.buckets[0].latencyCounts,
	}
	for i := 1; i < len(s.buckets); i++ {
		aggBucket.add(s.buckets[i].diff(s.buckets[i-1]))
	}

	// If there is a single bucket, total duration should be 1 minute, given
	// that the first snapshot happens 1 minute after the start of the job.
	totalDurationSec := 60.0
	if len(s.buckets) > 1 {
		totalDurationSec = lastBucket.time.Sub(s.buckets[0].time).Seconds()
	}
	result.Hour = methodStats{
		NumCalls:     aggBucket.calls,
		SentKBPerSec: aggBucket.kbSent / totalDurationSec,
		RecvKBPerSec: aggBucket.kbRecvd / totalDurationSec,
	}
	if aggBucket.latencyCounts > 0 {
		result.Hour.AvgLatencyMs = aggBucket.latencyMs / aggBucket.latencyCounts
	}
	return result
}
