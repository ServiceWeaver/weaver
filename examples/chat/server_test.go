// Copyright 2023 Google LLC
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
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/weavertest"
)

// db is a fake store for the chat database.
type db struct {
	mu       sync.Mutex
	lastPost PostID
	threads  []Thread
}

var _ SQLStore = &db{}

func (db *db) CreateThread(ctx context.Context, creator string, when time.Time, others []string, text string, image []byte) (ThreadID, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.lastPost++
	t := Thread{
		ID:    ThreadID(len(db.threads)) + 1,
		Posts: []Post{{ID: db.lastPost, Creator: creator, When: when, Text: text}}, // image is ignored
	}
	db.threads = append(db.threads, t)
	return t.ID, nil
}

func (db *db) CreatePost(ctx context.Context, creator string, when time.Time, thread ThreadID, text string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.lastPost++
	t := db.threads[int(thread-1)]
	t.Posts = append(t.Posts, Post{ID: db.lastPost, Creator: creator, When: when, Text: text})
	db.threads[int(thread-1)] = t
	return nil
}

func (db *db) GetFeed(ctx context.Context, user string) ([]Thread, error) {
	db.mu.Lock()
	defer db.mu.Unlock()

	// Deep copy results
	threads := make([]Thread, len(db.threads))
	for i, t := range db.threads {
		t.Posts = append([]Post(nil), t.Posts...)
		threads[i] = t
	}
	return threads, nil
}

func (db *db) GetImage(ctx context.Context, _ string, image ImageID) ([]byte, error) {
	return nil, fmt.Errorf("no images")
}

func TestServer(t *testing.T) {
	for _, r := range weavertest.AllRunners() {
		// Use a fake DB since normal implementation does not work with multiple replicas
		db := &db{}
		r.Fakes = []weavertest.FakeComponent{weavertest.Fake[SQLStore](db)}
		r.Test(t, func(t *testing.T, main *server) {
			server := httptest.NewServer(main)
			defer server.Close()

			// Login
			expect(t, "login", httpGet(t, server.URL, "/"), "Login")
			expect(t, "feed", httpGet(t, server.URL, "/?name=user"), "/newthread")

			// Create a new thread
			r := httpGet(t, server.URL, "/newthread?name=user&recipients=bar,baz&message=Hello&image=")
			expect(t, "new thread", r, "Hello")
			m := regexp.MustCompile(`id="tid(\d+)"`).FindStringSubmatch(r)
			if m == nil {
				t.Fatalf("no tid in /newthread respinse:\n%s\n", r)
			}
			tid := m[1]

			// Reply to thread
			r = httpGet(t, server.URL, fmt.Sprintf("/newpost?name=user&tid=%s&post=Hi!", tid))
			expect(t, "reply", r, `Hello`)
			expect(t, "reply", r, `Hi!`)
		})
	}
}

func httpGet(t *testing.T, addr, path string) string {
	t.Helper()
	response, err := http.Get(addr + path)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	result, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(result)
}

func expect(t *testing.T, title, subject, re string) {
	t.Helper()
	r := regexp.MustCompile(re)
	if !r.MatchString(subject) {
		t.Fatalf("did not find %q in %s:\n%s\n", re, title, subject)
	}
}
