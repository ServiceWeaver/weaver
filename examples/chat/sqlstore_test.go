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
)

func TestFeed(t *testing.T) {
	weavertest.Run(t, weavertest.Options{SingleProcess: true}, func(store SQLStore) {
		ctx := context.Background()

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

		const thread1 = "alice:msg3 bob:msg1"
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
	weavertest.Run(t, weavertest.Options{SingleProcess: true}, func(store SQLStore) {
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
