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

package logging

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/ServiceWeaver/weaver/internal/cond"
	"github.com/ServiceWeaver/weaver/internal/heap"
	"github.com/ServiceWeaver/weaver/runtime/colors"
	"github.com/ServiceWeaver/weaver/runtime/protomsg"
	"github.com/ServiceWeaver/weaver/runtime/protos"
	"github.com/fsnotify/fsnotify"
	"github.com/google/cel-go/cel"
)

// This file contains code to read and write log entries to and from files.

// DefaultLogDir is the default directory where Service Weaver log files are stored.
var DefaultLogDir = filepath.Join(os.TempDir(), "serviceweaver", "logs")

// FileStore stores log entries in files.
type FileStore struct {
	dir string
	mu  sync.Mutex
	pp  *PrettyPrinter

	// We segregate into log files by app,deployment,node,level.
	files map[string]*os.File
}

// NewFileStore returns a LogStore that writes files to the specified directory.
func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, err
	}
	return &FileStore{
		dir:   dir,
		pp:    NewPrettyPrinter(colors.Enabled()),
		files: map[string]*os.File{},
	}, nil
}

// Close closes the specified log-store, including any opened files.
func (fs *FileStore) Close() error {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	var err error
	for name, f := range fs.files {
		delete(fs.files, name)
		if f != nil {
			if fileErr := f.Close(); fileErr != nil && err == nil {
				err = fileErr
			}
		}
	}
	return err
}

// Add stores the specified log entry, assigning a timestamp to it if necessary.
func (fs *FileStore) Add(e *protos.LogEntry) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	// Assign timestamp while holding a lock to ensure we write in timestamp order.
	// For pre-assigned timestamps, assume that the caller has arranged for everything
	// that will end up in the same file to be ordered properly.
	if e.TimeMicros == 0 {
		e.TimeMicros = time.Now().UnixMicro()
	}

	// Get the log file, creating it if necessary.
	fname := filename(e.App, e.Version, e.Node, e.Level)
	f, ok := fs.files[fname]
	if !ok {
		var err error
		f, err = os.Create(filepath.Join(fs.dir, fname))
		if err != nil {
			// Since we can't open the log file, fall back to stderr.
			fmt.Fprintf(os.Stderr, "create log file: %v\n", err)
			f = nil
		}
		fs.files[fname] = f
	}

	// Write to log file if available.
	if f != nil {
		err := protomsg.Write(f, e)
		if err == nil {
			return
		}
		// Fall back to stderr.
		fmt.Fprintf(os.Stderr, "write log entry: %v\n", err)
		fs.files[fname] = nil
	}

	// Log file is not available, so write to stderr.
	fmt.Fprintln(os.Stderr, fs.pp.Format(e))
}

// filename returns the log file for the specified (app, deployment, weavelet,
// level) tuple.
//
// These files are typically stored in DefaultLogDir. The directory contains
// one log file for every (app, deployment, weavelet, level) tuple. For
// example, the logs directory might look like this:
//
//	/tmp/serviceweaver/logs
//	├── collatz.v1.111.info.log
//	├── collatz.v1.111.error.log
//	├── collatz.v1.222.log
//	├── todo.v1.111.info.log
//	└── todo.v2.111.error.log
//
// TODO(mwhittaker): Instead of this structure, we could instead have
// directories for every deployment. For example, we could have
// /tmp/serviceweaver/logs/todo/v1, /tmp/serviceweaver/logs/todo/v2, and so on. This makes
// catting logs cleaner (we don't have to look through every single log file
// and filter out the ones we're not interested in), but it makes tailing logs
// much more complicated. Another extreme is to store all logs in a single
// database. Reconsider the storage of logs after we have a better sense for
// the performance.
//
// TODO(mwhittaker): We store the application, deployment, weavelet id,
// level redundantly in every log entry. Omit them from the log entries and
// infer them from the log name.
func filename(app, deployment, weavelet, level string) string {
	return fmt.Sprintf("%s.%s.%s.%s.log", app, deployment, weavelet, level)
}

// logfile represents a log file for a specific (app, deployment, weavelet,
// level) tuple.
type logfile struct {
	app        string
	deployment string
	weavelet   string
	level      string
}

// parseLogfile parses a logfile filename.
func parseLogfile(filename string) (logfile, error) {
	// TODO(mwhittaker): Ensure that apps, deployments, weavelet ids, levels
	// don't contain a ".". Or, switch to some other delimiter that doesn't
	// show up.
	parts := strings.SplitN(filename, ".", 5)
	if len(parts) < 5 || parts[4] != "log" {
		want := "<app>.<deployment>.<weavelet>.<level>.log"
		return logfile{}, fmt.Errorf("filename %q must have format %q", filename, want)
	}
	return logfile{
		app:        parts[0],
		deployment: parts[1],
		weavelet:   parts[2],
		level:      parts[3],
	}, nil
}

// matches returns whether the provided compiled query may match some log
// entries in this logfile.
func (l *logfile) matches(prog cel.Program) (bool, error) {
	// Note that prog may query fields besides app, version, node, and
	// level. For example, a query might look like this:
	//
	//     msg == "a" && app == "todo"
	//
	// How do we evaluate such a query if we don't provide a msg? CEL does not
	// implement short circuting boolean algebra, and will instead try to
	// evaluate the query fully if possible. For example, if app is "foo", then
	// CEL will evaluate the previous query as
	//
	//     msg == "a" && app == "todo"   // original query
	//     ??? == "a" && "foo" == "todo" // substitute known values
	//     ??? == "a" && false           // simplify
	//     false                         // simplify
	//
	// If CEL cannot fully evaluate the query it will return an error. For
	// example, the query `msg == "a"` cannot be fully evaluated given only
	// app, deployment, and node, so it evaluates to an error.
	//
	// In summary, when we evaluate a query on a file, we have the following
	// situations:
	//
	//     | query evaluates to | meaning                                   |
	//     | ------------------ | ----------------------------------------- |
	//     | true               | every entry in the file matches           |
	//     | false              | none of the entries in the file match     |
	//     | error              | some of the entries in the file may match |
	out, _, err := prog.Eval(map[string]interface{}{
		"app":          l.app,
		"version":      Shorten(l.deployment),
		"full_version": l.deployment,
		"node":         Shorten(l.weavelet),
		"full_node":    l.weavelet,
		"level":        l.level,
	})

	// See [1] for an explanation of the values returned by Eval.
	//
	// [1]: https://pkg.go.dev/github.com/google/cel-go/cel#Program
	if out == nil && err != nil {
		// The evaluation was unsuccessful.
		return false, err
	} else if err != nil {
		// The evaluation was successful, but it resulted in an error. In this
		// case, the query _may_ match some log entries in the file, so we
		// return true. See above for more details.
		return true, nil
	}
	b, err := out.ConvertToNative(reflect.TypeOf(true))
	if err != nil {
		return false, err
	}
	return b.(bool), nil
}

// fileSource is a Source that reads logs from files produced by a FileStore.
type fileSource struct {
	dir string
}

var _ Source = &fileSource{}

// FileSource returns a new Source that reads logs from files saved by a
// FileStore.
func FileSource(dir string) Source {
	return &fileSource{dir: dir}
}

// Query implements the Source interface.
func (fq *fileSource) Query(_ context.Context, q Query, follow bool) (Reader, error) {
	if err := os.MkdirAll(fq.dir, 0750); err != nil {
		return nil, err
	}

	if follow {
		return newFileFollower(fq.dir, q)
	}
	return newFileCatter(fq.dir, q)
}

// A fileCatter performs a streaming heap sort on the set of files that match
// its query, sorting them based on the timestamps of their log entries. This
// is similar to how you might implement part of a sort-merge join in a
// relational database.
//
// For example, imagine you have three files A, B, and C with the following
// entries (only timestamps shown):
//
//     A: 0
//     B: 1, 4, 5
//     C: 2, 3
//
// A fileCatter forms a heap of the three files, sorted by the timestamp of
// their first entry. The fileCatter pops its topmost entry, A, and cats its
// log entry (0). A doesn't have any more log entries, so it's discarded. Next,
// the fileCatter pops B, cats its log entry (1), and then pushes B back on to
// the heap because it has more entries to read. The fileCatter then pops and
// pushes C, catting entry 2. It then pops C, cats entry 3, and discards C.
// Finally, it pops, pushes, pops, and discards B, catting entries 4 and 5. At
// this point, heap is empty, and the fileCatter is done.
//
// Note that this implementation assumes that the entries in the log files are
// sorted by timestamp. This is guaranteed to be true for files generated by a
// fileLogger.

// fileCatter is a Reader implementation that reads from files written by a
// FileLogger.
type fileCatter struct {
	prog   cel.Program           // query for filtering log entries
	h      *heap.Heap[*buffered] // heap of *buffered
	files  []*os.File            // underlying files being read
	closed bool                  // true if Close() has been called
}

func newFileCatter(logdir string, q Query) (*fileCatter, error) {
	// Compile the query.
	env, ast, err := parse(q)
	if err != nil {
		return nil, err
	}
	prog, err := compile(env, ast)
	if err != nil {
		return nil, err
	}

	// Construct the heap.
	h := heap.New(func(a, b *buffered) bool {
		return a.peek().TimeMicros < b.peek().TimeMicros
	})
	filenames, err := ls(logdir, prog)
	if err != nil {
		return nil, err
	}
	files := make([]*os.File, 0, len(filenames))
	for _, filename := range filenames {
		// TODO(mwhittaker): Close this file if we return an error.
		file, err := os.Open(filepath.Join(logdir, filename))
		if err != nil {
			return nil, err
		}
		files = append(files, file)

		buffered := newBuffered(file.Name(), file)
		if err = buffered.buffer(); err != nil {
			return nil, err
		}
		if buffered.peek() != nil {
			h.Push(buffered)
		}
	}
	catter := fileCatter{
		prog:   prog,
		h:      h,
		files:  files,
		closed: false,
	}
	return &catter, nil
}

// Read implements the Reader interface.
func (fc *fileCatter) Read(ctx context.Context) (*protos.LogEntry, error) {
	if fc.closed {
		return nil, fmt.Errorf("closed")
	}

	for ctx.Err() == nil {
		buffered, ok := fc.h.Pop()
		if !ok {
			return nil, io.EOF
		}
		entry := buffered.pop()

		// Update the scanner.
		if err := buffered.buffer(); err != nil {
			return nil, err
		}
		if buffered.peek() != nil {
			fc.h.Push(buffered)
		}

		// Check the entry against our query.
		matches, err := matches(fc.prog, entry)
		if err != nil {
			return nil, err
		}
		if matches {
			return entry, nil
		}
	}
	return nil, ctx.Err()
}

// Close implements the Reader interface.
func (fc *fileCatter) Close() {
	if fc.closed {
		return
	}
	fc.closed = true
	for _, file := range fc.files {
		file.Close()
	}
}

// A fileFollower, like a fileCatter, performs a streaming heap sort on the set
// of files that match its query. A fileFollower is more complicated, however,
// for two reasons:
//
//     1. A new log file might appear, and the fileFollower has to detect it
//        and start following it. For example, if we construct a follower with
//        query `app=="todo"`, it may initially find logs for the v1
//        deployment. If later a v2 deployment is rolled out, the follower has
//        to detect those logs and start following them.
//     2. A fileFollower has to follow logs, not just cat them. If the
//        fileFollower hits an EOF on a log file, it has to stop reading. But,
//        if the file is later written to, the fileFollower has to read in the
//        new data.
//
// A fileFollower addresses issue (1) by using an fsnotify.Watcher. A Watcher
// watches a file system directory and is triggered whenever (a) a file is
// created in the directory or (b) a file in the directory is written to. A
// fileFollower addresses issue (2) by using a tailReader that is hooked up to
// the Watcher. Whenever the Watcher detects that a file has been written to,
// it wakes up the tailReader that is reading the file (if it's blocked).
//
// Details
//
// A fileFollower launches one goroutine for every file that it follows (see
// fileFollower.scan). Every goroutine uses a tailReader to scan log entries
// from the file, discarding those that don't match the user provided query.
// Every goroutine also has a corresponding *fileScanner that stores a small
// buffer of the scanned entries that do match the query.
//
// A fileFollower maintains a heap of *fileScanner. A fileScanner is on the
// heap if and only if it has at least one buffered log entry. The fileScanners
// on the heap are sorted by the timestamp of the next buffered log entry. If a
// fileScanner is not on the heap and its corresponding goroutine is blocked on
// an EOF, we call the fileScanner pending.
//
// Let H be the number of fileScanners on the heap, let P be the number of
// pending fileScanners, and let N be the total number of files a fileFollower
// is following. fileFollower.Read waits for H > 0 and H + P == N. When this
// condition is true, every fileScanner either (a) has a buffered log entry
// ready to go, or (b) is blocked waiting for a new log entry to appear.
// fileFollower.Read then pops a fileScanner off the heap, updates its state,
// and returns the popped fileScanner's buffered log entry.
//
// A fileFollower also launches a single fsnotify.Watcher in its own goroutine
// that watches logdir. Whenever the Watcher reports that a new log file has
// been created, if the file matches our query, the fileFollower launches a
// scanning goroutine and creates a corresponding fileScanner. When the
// Watcher reports that a file has been written to, the fileFollower signals
// the corresponding tailReader using a condition variable.

// fileFollower is a Reader implementation that reads from files written by a
// FileLogger.
type fileFollower struct {
	prog cel.Program // the compiled user provided query

	mu         sync.Mutex               // guards the following fields
	scanners   map[string]*fileScanner  // all scanners, keyed by filename
	h          *heap.Heap[*fileScanner] // heap of *fileScanner with non-nil entry
	numPending int                      // the number of pending scanners
	ready      cond.Cond                // signalled when isReady returns true
	closed     bool                     // true if Close has been called
	err        error                    // an error encountered by a goroutine

	watcher *fsnotify.Watcher  // watches logdir
	ctx     context.Context    // context used by all goroutines
	cancel  context.CancelFunc // cancels ctx
	done    sync.WaitGroup     // waits for all goroutines to terminate
}

type fileScanner struct {
	file    *os.File              // file being scanned
	entry   *protos.LogEntry      // buffered entry scanned from scanner
	buf     chan *protos.LogEntry // buffer of entries scanned from scanner
	blocked bool                  // is tailReader blocked?
	reader  *tailReader           // reads the file
	ready   *cond.Cond            // signals reader that more bytes are ready
}

func (fs *fileScanner) onHeap() bool {
	return fs.entry != nil
}

func newFileFollower(logdir string, q Query) (*fileFollower, error) {
	// Compile the query.
	env, ast, err := parse(q)
	if err != nil {
		return nil, err
	}
	prog, err := compile(env, ast)
	if err != nil {
		return nil, err
	}

	// Start watching logdir. Note that we have to start watching logdir before
	// we call ls. Otherwise, in the gap between calling ls and starting
	// watching logdir, we may miss a file creation.
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err := watcher.Add(logdir); err != nil {
		return nil, err
	}

	// Construct the follower.
	ctx, cancel := context.WithCancel(context.Background())
	follower := fileFollower{
		prog: prog,

		scanners: map[string]*fileScanner{},
		h: heap.New(func(a, b *fileScanner) bool {
			return a.entry.TimeMicros < b.entry.TimeMicros
		}),
		numPending: 0,
		closed:     false,
		err:        nil,

		watcher: watcher,
		ctx:     ctx,
		cancel:  cancel,
	}
	follower.ready.L = &follower.mu

	// Add all existing files. If any of these files were created after the
	// watcher started watching, then we'll also get a notification from the
	// watcher, but that's okay.
	filenames, err := ls(logdir, prog)
	if err != nil {
		return nil, err
	}
	for _, filename := range filenames {
		abs := filepath.Join(logdir, filename)
		if err := follower.created(abs); err != nil {
			return nil, err
		}
	}

	// Start the watcher goroutine.
	follower.spawn(func() error { return follower.watch(ctx) })

	return &follower, nil
}

// Read implements the Reader interface.
func (ff *fileFollower) Read(ctx context.Context) (*protos.LogEntry, error) {
	ff.mu.Lock()
	defer ff.mu.Unlock()

	if ff.closed {
		return nil, fmt.Errorf("closed")
	}
	if ff.err != nil {
		return nil, ff.err
	}

	for ctx.Err() == nil {
		// Wait for isReady() to be true.
		for !ff.isReady() {
			if err := ff.ready.Wait(ctx); err != nil {
				return nil, err
			}
		}

		// Check for errors from the watcher goroutine.
		if ff.err != nil {
			return nil, ff.err
		}

		// Read from the heap.
		for ctx.Err() == nil {
			scanner, ok := ff.h.Pop()
			if !ok {
				break
			}
			entry := scanner.entry
			scanner.entry = nil

			select {
			case scanner.entry = <-scanner.buf:
				ff.h.Push(scanner)
			default:
				if scanner.blocked {
					ff.numPending += 1
				}
			}

			return entry, nil
		}
	}
	return nil, ctx.Err()
}

// Close implements the Reader interface.
func (ff *fileFollower) Close() {
	if ff.closed {
		return
	}
	ff.closed = true
	ff.cancel()
	ff.done.Wait()
	for _, scanner := range ff.scanners {
		scanner.file.Close()
	}
}

// isReady returns true if either (a) the heap is non-empty and every scanner
// is accounted for (i.e. H + P == N) or (b) ff.err is not nil. In either case,
// fileFollower.Read is ready to act and should be woken up.
func (ff *fileFollower) isReady() bool {
	return (ff.h.Len() > 0 && ff.h.Len()+ff.numPending == len(ff.scanners)) || ff.err != nil
}

// spawn runs f in its own goroutine.
func (ff *fileFollower) spawn(f func() error) {
	ff.done.Add(1)
	go func() {
		defer ff.done.Done()
		err := f()
		if err == nil || errors.Is(err, ff.ctx.Err()) {
			return
		}
		ff.mu.Lock()
		defer ff.mu.Unlock()
		if ff.err != nil {
			ff.err = err
			ff.ready.Signal()
		}
	}()
}

// created updates a fileFollower with a file it may have never seen before.
func (ff *fileFollower) created(filename string) error {
	ff.mu.Lock()
	defer ff.mu.Unlock()

	// We've already seen this file before. This is only possible when our call
	// to ls in newFileFollower races ff.watcher and they both report the same
	// file. We make sure not to process the same file twice.
	if _, seen := ff.scanners[filename]; seen {
		return nil
	}

	// Check to see if we need to watch this file.
	logfile, err := parseLogfile(filepath.Base(filename))
	if err != nil {
		return err
	}
	b, err := logfile.matches(ff.prog)
	if err != nil {
		return err
	}
	if !b {
		return nil
	}

	// Open the file.
	file, err := os.Open(filename)
	if err != nil {
		return err
	}

	// Make a tailReader for the file.
	fs := &fileScanner{
		file:    file,
		entry:   nil,
		buf:     make(chan *protos.LogEntry, 10),
		blocked: false,
		reader:  nil,
		ready:   cond.NewCond(&ff.mu),
	}
	reader := newTailReader(file, func() error { return ff.waitForChanges(fs) })
	fs.reader = reader
	ff.scanners[filename] = fs

	// Launch a goroutine that scans the file.
	ff.spawn(func() error { return ff.scan(fs) })
	return nil
}

// waitForChanges blocks on fs.ready, waiting for more bytes to be written to
// fs.file. The watcher goroutine (see the watch method) will signal fs.ready
// when it detects that the file has been written to.
func (ff *fileFollower) waitForChanges(fs *fileScanner) error {
	ff.mu.Lock()
	defer ff.mu.Unlock()

	fs.blocked = true
	if !fs.onHeap() {
		// The fileScanner is not on the heap, and now it's blocked, so
		// it's pending.
		ff.numPending += 1
	}
	if ff.isReady() {
		ff.ready.Signal()
	}

	// This Wait is not guarded by a for loop. It can spuriously wake up.
	// That is okay. The function passed to newTailReader can spuriously
	// return without causing problems.
	err := fs.ready.Wait(ff.ctx)

	fs.blocked = false
	if !fs.onHeap() {
		// The fileScanner is not on the heap, and now it's not
		// blocked, so it's no longer pending.
		ff.numPending -= 1
	}
	return err
}

// scan repeatedly scans and buffers entries.
func (ff *fileFollower) scan(fs *fileScanner) error {
	entry := &protos.LogEntry{} // Reuse log-entry until we yield it.
	for ff.ctx.Err() == nil {
		// Scan a log entry.
		//
		// Note that fs.reader is cancelled when ff.ctx is cancelled. This will
		// also cause scanner.Scan to be cancelled.
		err := protomsg.Read(fs.reader, entry)
		if err != nil {
			return err
		}
		b, err := matches(ff.prog, entry)
		if err != nil {
			return err
		}
		if !b {
			// The entry doesn't match our query.
			continue
		}

		// Buffer the log entry.
		select {
		case <-ff.ctx.Done():
			return ff.ctx.Err()
		case fs.buf <- entry:
			entry = &protos.LogEntry{}
		}

		// Update the heap. If fs is not currently on the heap, you might
		// expect that tryPush will always successfully push fs onto the heap
		// because we just buffered an entry into fs.buf. However, this is not
		// the case. Between buffering entry and calling tryPush, the Read
		// method may have already popped and returned the entry, leaving fs
		// without any buffered entries.
		ff.tryPush(fs)
	}
	return ff.ctx.Err()
}

// tryPush pushes fs onto the heap if it's eligible to be on the heap and if
// isn't already on the heap.
func (ff *fileFollower) tryPush(fs *fileScanner) {
	ff.mu.Lock()
	defer ff.mu.Unlock()
	if fs.onHeap() {
		return
	}

	select {
	case fs.entry = <-fs.buf:
		ff.h.Push(fs)
		if fs.blocked {
			// fs was pending, but now that it's on the heap, it no longer is.
			ff.numPending -= 1
		}
		if ff.isReady() {
			ff.ready.Signal()
		}

	default:
		// fs doesn't have any buffered entries, so we cannot push it onto the
		// heap.
	}
}

// written updates a fileFollower with a recently written file.
func (ff *fileFollower) written(filename string) {
	fs, ok := ff.scanners[filename]
	if !ok {
		return
	}
	fs.ready.Signal()
}

// watch watches for updates to logdir. If a file is created or written to, it
// is passed to the created or written method.
func (ff *fileFollower) watch(ctx context.Context) error {
	defer ff.watcher.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case event := <-ff.watcher.Events:
			switch event.Op {
			case fsnotify.Remove, fsnotify.Rename, fsnotify.Chmod:
				return fmt.Errorf("unexpected operation %v", event.Op)

			case fsnotify.Create:
				if err := ff.created(event.Name); err != nil {
					return fmt.Errorf("created(%q): %w", event.Name, err)
				}

			case fsnotify.Write:
				ff.written(event.Name)
			}

		case err := <-ff.watcher.Errors:
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				// See [1] for a description on how fsnotify works and what an
				// overflow means. In short, when we create an inotify
				// instance, the kernel allocates a queue of events. The kernel
				// pushes events on to the queue, and we pop events off of the
				// queue. If the queue becomes too big, then the kernel pushes
				// an overflow event on to the queue and starts dropping events
				// until the queue has been sufficiently drained.
				//
				// [1] discusses how to handle overflow events. Unfortunately,
				// it's complicated and is one of the shortcomings of inotify.
				// For our use case, we simply ignore the overflow. If the
				// queue is overflowing, it's likely that a lot of log entries
				// are being written. In this case, ignoring some of the Write
				// events is likely fine.
				//
				// TODO(mwhittaker): Handle overflows in a more principled way.
				// For example, we can have a very slow poll to catch any
				// updates that were dropped as part of an overflow.
				//
				// [1]: https://lwn.net/Articles/605128
				continue
			}
			return fmt.Errorf("fsnotify error: %w", err)
		}
	}
}

// ls returns the set of filenames in dir that match the provided query.
func ls(dir string, prog cel.Program) ([]string, error) {
	direntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	filenames := make([]string, 0, len(direntries))
	for _, direntry := range direntries {
		if direntry.IsDir() {
			return nil, fmt.Errorf("unexpected directory %q in %q", direntry.Name(), dir)
		}
		filename := direntry.Name()
		logfile, err := parseLogfile(filename)
		if err != nil {
			return nil, err
		}
		matches, err := logfile.matches(prog)
		if err != nil {
			return nil, err
		}
		if matches {
			filenames = append(filenames, filename)
		}
	}
	return filenames, nil
}

// buffered is an entryScanner with a buffered *Entry scanned from it.
type buffered struct {
	filename string           // absolute filename of the file being scanned
	entry    *protos.LogEntry // entry scanned from scanner
	src      *bufio.Reader    // source of log entries
}

// newBuffered returns a new buffered.
func newBuffered(filename string, src io.Reader) *buffered {
	return &buffered{
		filename: filename,
		entry:    nil,
		src:      bufio.NewReader(src),
	}
}

// buffer tries to buffer an entry from the underlying scanner, if there isn't
// already a buffered entry. If the scanner encounters an EOF, nil is returned.
// A non-nil error is only returned if something unrecoverable happens.
func (b *buffered) buffer() error {
	if b.entry != nil {
		return nil
	}

	entry := &protos.LogEntry{}
	err := protomsg.Read(b.src, entry)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	} else if errors.Is(err, io.EOF) {
		return nil
	}
	b.entry = entry
	return nil
}

// peek returns the buffered entry, or nil if there is no buffered entry.
func (b *buffered) peek() *protos.LogEntry {
	return b.entry
}

// pop returns and unbuffers the buffered entry, or returns nil if there is no
// buffered entry.
func (b *buffered) pop() *protos.LogEntry {
	entry := b.entry
	b.entry = nil
	return entry
}
