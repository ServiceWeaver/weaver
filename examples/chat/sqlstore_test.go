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
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ServiceWeaver/weaver/weavertest"
	"github.com/google/go-cmp/cmp"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/mysql"
)

// newMySQL launches a new, fully initialized MySQL instance suitable for use
// with a sqlStore. newMySQL returns the connection string to connect to the
// MySQL instance, and it tears down the instance when the provided test ends.
func newMySQL(t *testing.T, ctx context.Context) string {
	t.Helper()

	// Create the container.
	container, err := mysql.RunContainer(ctx,
		testcontainers.WithImage("mysql"),
		mysql.WithDatabase("chat"),
		mysql.WithScripts("chat.sql"),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Clean up the container when the test ends.
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Fatal(err)
		}
	})

	// Return the connection string.
	addr, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return addr
}

func TestFeed(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)

	// Start a MySQL instance.
	ctx := context.Background()
	addr := newMySQL(t, ctx)

	// Run the test against the instance.
	runner := weavertest.Local
	runner.Config = fmt.Sprintf(`
		["github.com/ServiceWeaver/weaver/examples/chat/SQLStore"]
		db_driver = "mysql"
		db_uri = %q
	`, addr)
	runner.Test(t, func(t *testing.T, store SQLStore) {
		// Run the test.
		makeThread := func(user, msg string, others ...string) ThreadID {
			tid, err := store.CreateThread(ctx, user, time.Now(), others, msg, nil)
			if err != nil {
				t.Fatal(err)
			}
			return tid
		}
		post := func(tid ThreadID, user, msg string) {
			err := store.CreatePost(ctx, user, time.Now(), tid, msg)
			if err != nil {
				t.Fatal(err)
			}
		}
		threadContents := func(thread Thread) string {
			var parts []string
			for _, post := range thread.Posts {
				parts = append(parts, fmt.Sprintf("%s:%s", post.Creator, post.Text))
			}
			return strings.Join(parts, " ")
		}

		t1 := makeThread("bob", "msg1", "alice", "ted")
		t2 := makeThread("ted", "msg2", "bob")
		post(t1, "alice", "msg3")
		post(t2, "ted", "msg4")

		const thread1 = "bob:msg1 alice:msg3"
		const thread2 = "ted:msg2 ted:msg4"
		for user, expect := range map[string][]string{
			"bob":   {thread2, thread1},
			"ted":   {thread2, thread1},
			"alice": {thread1},
		} {
			t.Run(user, func(t *testing.T) {
				feed, err := store.GetFeed(ctx, user)
				if err != nil {
					t.Fatalf("error getting feed for %s: %v", user, err)
				}
				var result []string
				for _, thread := range feed {
					result = append(result, threadContents(thread))
				}
				if diff := cmp.Diff(expect, result); diff != "" {
					t.Errorf("GetFeed(%s): (-want,+got):\n%s\n", user, diff)
				}
			})
		}
	})
}

func TestImage(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)

	// Start a MySQL instance.
	ctx := context.Background()
	addr := newMySQL(t, ctx)

	// Run the test against the instance.
	runner := weavertest.Local
	runner.Config = fmt.Sprintf(`
		["github.com/ServiceWeaver/weaver/examples/chat/SQLStore"]
		db_driver = "mysql"
		db_uri = %q
	`, addr)
	runner.Test(t, func(t *testing.T, store SQLStore) {
		ctx := context.Background()

		// Create thread with an image in the initial post.
		img := "test image"
		tid, err := store.CreateThread(ctx, "a", time.Now(), []string{"b"}, "foo", []byte(img))
		if err != nil {
			t.Fatal(err)
		}

		// Add a reply (without an image).
		err = store.CreatePost(ctx, "b", time.Now(), tid, "bar")
		if err != nil {
			t.Fatal(err)
		}

		for _, user := range []string{"a", "b"} {
			t.Run(user, func(t *testing.T) {
				feed, err := store.GetFeed(ctx, user)
				if err != nil {
					t.Fatalf("could not get feed: %v", err)
				}
				if len(feed) != 1 {
					t.Fatalf("expected one thread, got %d", len(feed))
				}
				thread := feed[0]
				if len(thread.Posts) != 2 {
					t.Fatalf("expected two posts, got %d", len(thread.Posts))
				}
				if thread.Posts[0].ImageID == 0 {
					t.Errorf("initial post did not contain expected image")
				}
				if thread.Posts[1].ImageID == 1 {
					t.Errorf("reply contained unexpected image")
				}
				got, err := store.GetImage(ctx, user, ImageID(thread.Posts[0].ImageID))
				if err != nil {
					t.Fatalf("could not read image contents")
				}
				if string(got) != img {
					t.Errorf("wrong image contents")
				}
			})
		}
	})
}
