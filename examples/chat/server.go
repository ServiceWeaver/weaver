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
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ServiceWeaver/weaver"
)

type server struct {
	weaver.Implements[weaver.Main]
	httpServer http.Server
	store      weaver.Ref[SQLStore]
	scaler     weaver.Ref[ImageScaler]
	cache      weaver.Ref[LocalCache]
	chat       weaver.Listener
}

func serve(ctx context.Context, s *server) error {
	s.httpServer.Handler = instrument(s.label, s)
	s.Logger().Debug("Chat service available", "address", s.chat)
	return s.httpServer.Serve(s.chat)
}

// instrument instruments the provided handler with weaver.InstrumentHandler.
// The label for each request is determined by the provided labeler.
func instrument(labeler func(r *http.Request) string, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		weaver.InstrumentHandler(labeler(r), handler).ServeHTTP(w, r)
	})
}

func (s *server) label(r *http.Request) string {
	if r.URL.Query().Get("name") == "" {
		return "login"
	}

	switch r.URL.Path {
	case "/", "/thumbnail", "/newthread", "/newpost", weaver.HealthzURL:
		return r.URL.Path
	default:
		return "unknown"
	}
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// If user not specified, generate a "login" page where user name can be entered.
	user := r.URL.Query().Get("name")
	if user == "" {
		s.login(w)
		return
	}

	switch r.URL.Path {
	case "/":
		s.generateFeed(w, r, user)
	case "/thumbnail":
		s.serveThumbnail(w, r, user)
	case "/newthread":
		s.newThread(w, r, user)
	case "/newpost":
		s.newPost(w, r, user)
	case weaver.HealthzURL:
		// Returns OK status.
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (s *server) login(w http.ResponseWriter) {
	var data struct{}

	w.Header().Set("Content-Type", "text/html")
	err := loginTemplate.Execute(w, data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) redirectToFeed(w http.ResponseWriter, r *http.Request, user string) {
	q := url.Values{}
	q.Set("name", user)
	u := url.URL{Path: "/", RawQuery: q.Encode()}
	http.Redirect(w, r, u.String(), http.StatusSeeOther)
}

func (s *server) generateFeed(w http.ResponseWriter, r *http.Request, user string) {
	ctx := r.Context()
	threads, err := s.store.Get().GetFeed(ctx, user)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type feed struct {
		User    string
		Threads []Thread
	}
	f := feed{
		User:    user,
		Threads: threads,
	}
	err = feedTemplate.Execute(w, f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *server) serveThumbnail(w http.ResponseWriter, r *http.Request, user string) {
	ctx := r.Context()
	q := r.URL.Query()
	key := q.Get("id")
	if thumb, err := s.cache.Get().Get(ctx, key); err == nil {
		w.Write([]byte(thumb))
		return
	}
	id, err := getID(key)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	img, err := s.store.Get().GetImage(ctx, user, ImageID(id))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	const maxSize = 128
	small, err := s.scaler.Get().Scale(ctx, img, maxSize, maxSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write(small)
	_ = s.cache.Get().Put(ctx, key, string(small))
}

func (s *server) newThread(w http.ResponseWriter, r *http.Request, user string) {
	others, err := getOthers(user, r.FormValue("recipients"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	msg := r.FormValue("message")
	if msg == "" {
		http.Error(w, "no message", http.StatusBadRequest)
		return
	}
	img, err := getImage(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	_ = img

	_, err = s.store.Get().CreateThread(r.Context(), user, time.Now(), others, msg, img)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.redirectToFeed(w, r, user)
}

func (s *server) newPost(w http.ResponseWriter, r *http.Request, user string) {
	tid, err := getID(r.FormValue("tid"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	msg := r.FormValue("post")
	if msg == "" {
		http.Error(w, "no message", http.StatusBadRequest)
		return
	}
	err = s.store.Get().CreatePost(r.Context(), user, time.Now(), ThreadID(tid), msg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.redirectToFeed(w, r, user)
}

var (
	validUserRE = regexp.MustCompile(`^\w+$`)
	sepRE       = regexp.MustCompile(`\s|,`)
)

func getOthers(me string, recipients string) ([]string, error) {
	var result []string
	for _, user := range sepRE.Split(recipients, -1) {
		user = strings.TrimSpace(user)
		if user == "" {
			continue
		}
		if user == me {
			continue
		}
		if err := validateUser(user); err != nil {
			return nil, err
		}
		result = append(result, user)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no other recipients")
	}
	return result, nil
}

func getID(str string) (int64, error) {
	id, err := strconv.ParseInt(str, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id %q", str)
	}
	return id, nil
}

// getImage parses image from the specified request. It returns a non-nil error if
// there is a malformed image, nil,nil if no image is specified, or image contents
// if the image is present.
func getImage(r *http.Request) ([]byte, error) {
	img, fhdr, err := r.FormFile("image")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) || errors.Is(err, http.ErrNotMultipart) {
			return nil, nil
		}
		return nil, fmt.Errorf("bad image form value %w", err)
	}

	// Validate by checking size and parsing the image.
	const maxSize = 128 << 10 // To avoid storing very large images.
	if fhdr.Size > maxSize {
		return nil, fmt.Errorf("image size %d is too large", fhdr.Size)
	}
	if _, _, err := image.Decode(img); err != nil {
		return nil, err
	}

	// Read image contents.
	if _, err = img.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(img)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func validateUser(user string) error {
	if !validUserRE.MatchString(user) {
		return fmt.Errorf("invalid user name %q", user)
	}
	return nil
}

var (
	//go:embed templates/login.html
	loginPage     string
	loginTemplate = template.Must(template.New("login").Parse(loginPage))

	//go:embed templates/feed.html
	feedPage     string
	feedTemplate = template.Must(template.New("feed").Parse(feedPage))
)
