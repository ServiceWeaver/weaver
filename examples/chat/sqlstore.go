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
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ServiceWeaver/weaver"
	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

// sqlStore provides storage for chat application data.
//
// The store holds:
// 1. A set of threads. Each thread contains some posts and  is shared with a
//
//	set of users.
//
// 2. A set of posts. Each post belongs to a thread, and contains the text of the
//
//	post as well as metadata (like the post creator).
//
// Threads and posts are identified by unique numeric IDs.
type sqlStore struct {
	weaver.Implements[SQLStore]
	weaver.WithConfig[config]
	db *sql.DB
}

// ThreadID uniquely identifies a thread in a particular sqlStore.
type ThreadID int64

// PostID uniquely identifies a post in a particular sqlStore.
type PostID int64

// ImageID identifies an image in the sqlStore. A zero value means no image is present.
type ImageID int64

// Thread holds information about a given thread.
type Thread struct {
	weaver.AutoMarshal
	ID    ThreadID
	Posts []Post
}

// Post holds information about a given post.
type Post struct {
	weaver.AutoMarshal
	ID      PostID
	Creator string
	When    time.Time
	Text    string
	ImageID ImageID
}

type SQLStore interface {
	CreateThread(ctx context.Context, creator string, when time.Time, others []string, text string, image []byte) (ThreadID, error)
	CreatePost(ctx context.Context, creator string, when time.Time, thread ThreadID, text string) error
	GetFeed(ctx context.Context, user string) ([]Thread, error)
	GetImage(ctx context.Context, _ string, image ImageID) ([]byte, error)
}

var (
	_ weaver.NotRetriable = SQLStore.CreateThread
	_ weaver.NotRetriable = SQLStore.CreatePost
)

type config struct {
	Driver string `toml:"db_driver"` // Name of the database driver.
	URI    string `toml:"db_uri"`    // Database server URI.
}

func (cfg *config) Validate() error {
	if cfg.Driver != "" {
		if len(cfg.URI) == 0 {
			return fmt.Errorf("DB driver specified but not location of database")
		}
	}
	return nil
}

func (s *sqlStore) Init(ctx context.Context) error {
	cfg := s.Config()
	if cfg.Driver == "" {
		return fmt.Errorf("missing database driver in config")
	}
	if cfg.URI == "" {
		return fmt.Errorf("missing database URI in config")
	}
	db, err := sql.Open(cfg.Driver, cfg.URI)
	if err != nil {
		return fmt.Errorf("error opening %q database %s: %w", cfg.Driver, cfg.URI, err)
	}
	if err := db.Ping(); err != nil {
		return fmt.Errorf("error pinging %q database %s: %w", cfg.Driver, cfg.URI, err)
	}
	s.db = db
	return nil
}

// CreateThread makes a new thread that is shared between creator and others
// and has an initial post containing the specified text, and an optional image.
// Returns the identifier for the new thread.
func (s *sqlStore) CreateThread(ctx context.Context, creator string, when time.Time, others []string, text string, image []byte) (ThreadID, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	// Create thread.
	r, err := tx.ExecContext(ctx, `INSERT INTO threads(creator) values(?)`, creator)
	if err != nil {
		return 0, err
	}
	tid, err := r.LastInsertId()
	if err != nil {
		return 0, err
	}

	// Save image if available and get its id.
	var imageID sql.NullInt64
	if len(image) != 0 {
		r, err := tx.ExecContext(ctx, `INSERT INTO images(image) values(?)`, image)
		if err != nil {
			return 0, err
		}
		id, err := r.LastInsertId()
		if err != nil {
			return 0, err
		}
		imageID.Int64 = id
		imageID.Valid = true
	}

	// Create initial post.
	_, err = tx.ExecContext(ctx, `INSERT INTO posts(thread,creator,time,text,imageid) values(?,?,?,?,?)`,
		tid, creator, when.Unix(), text, imageID)
	if err != nil {
		return 0, err
	}

	// Add thread into threadlist of all participants.
	for _, u := range append([]string{creator}, others...) {
		_, err = tx.ExecContext(ctx, `INSERT INTO userthreads(user,thread) values(?,?)`, u, tid)
		if err != nil {
			return 0, err
		}
	}

	err = tx.Commit()
	tx = nil
	return ThreadID(tid), err
}

// CreatePost adds a post to an existing thread.
func (s *sqlStore) CreatePost(ctx context.Context, creator string, when time.Time, thread ThreadID, text string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO posts(thread,creator,time,text) values(?,?,?,?)`,
		thread, creator, when.Unix(), text)
	return err
}

// GetFeed returns the list of threads and posts for the specified user.
func (s *sqlStore) GetFeed(ctx context.Context, user string) ([]Thread, error) {
	const query = `
SELECT u.thread, p.post, p.creator, p.time, p.text, p.imageid
FROM userthreads AS u
JOIN posts as p ON p.thread=u.thread
WHERE u.user=?
ORDER BY p.time, u.thread DESC;
`
	rows, err := s.db.QueryContext(ctx, query, user)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Thread
	for rows.Next() {
		var tid, pid, when int64
		var postCreator, text string
		var imageID sql.NullInt64
		err := rows.Scan(&tid, &pid, &postCreator, &when, &text, &imageID)
		if err != nil {
			return nil, err
		}

		n := len(result)
		if n == 0 || result[n-1].ID != ThreadID(tid) {
			// Start new thread.
			result = append(result, Thread{ID: ThreadID(tid)})
			n++
		}
		var iid ImageID
		if imageID.Valid {
			iid = ImageID(imageID.Int64)
		}

		result[n-1].Posts = append(result[n-1].Posts, Post{
			ID:      PostID(pid),
			Creator: postCreator,
			When:    time.Unix(when, 0),
			Text:    text,
			ImageID: iid,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetImage returns the image with the specified id.
func (s *sqlStore) GetImage(ctx context.Context, _ string, image ImageID) ([]byte, error) {
	const query = `
SELECT image
FROM images
WHERE images.id=?
LIMIT 1
`
	var img []byte
	err := s.db.QueryRowContext(ctx, query, image).Scan(&img)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("image not found")
	}
	if err != nil {
		return nil, err
	}
	return img, nil
}
