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

// docgen generates a static web site from markdown and other files.
//
// Usage:
//
//	go run dev/docgen/docgen.go
//
// The generated web site is placed under website/public. The following
// types of files can be generated:
//
//  1. Copy a file directly (see `staticFile` below).
//  2. A markdown file can be converted to HTML and the result passed to a
//     Go html/template.
//  3. An HTML file can be pre-processed and the result passed to a Go
//     html/template. The HTML file can contain embedded markdown
//     formatted as <markdown>...</markdown> regions. Each such region
//     is replaced with the region contents converted markdown to
//     HTML.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/fsnotify/fsnotify"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

var watch = flag.Bool("watch", false, "Automatically rebuild website on change")

var files = []file{
	{dst: "index.html", html: "index.html", template: "home.html", license: true},
	{dst: "docs.html", markdown: "docs.md", template: "docs.html", license: true},
	{dst: "examples.html", html: "examples.html", template: "examples.html", license: true},
	{dst: "contact.html", markdown: "contact.md", template: "contact.html", license: true},

	{dst: "blog/index.html", html: "blog/index.html", template: "blog.html", license: true},
	{dst: "blog/quick_intro.html", markdown: "blog/quick_intro.md", template: "blog_entry.html", title: "A Quick Introduction to Service Weaver", license: true},
	{dst: "news.html", html: "news.html", template: "news.html", license: true},
	{dst: "workshops.html", html: "workshops.html", template: "workshops.html", license: true},

	{dst: "blog/deployers.html", markdown: "blog/deployers.md", template: "blog_entry.html", title: "How to Implement a Service Weaver Deployer", license: true},
	staticFile("blog/deployers/assets/overview.svg"),
	staticFile("blog/deployers/assets/weavelets.svg"),

	{dst: "blog/corba.html", markdown: "blog/corba.md", template: "blog_entry.html", title: "CORBA vs the Fallacies of Distributed Computing", license: true},
	{dst: "blog/vision.html", markdown: "blog/vision.md", template: "blog_entry.html", title: "Monolith or Microservices or Both", license: true},

	staticFile("favicon.ico"),
	staticFile("assets/css/blog.css"),
	staticFile("assets/css/common.css"),
	staticFile("assets/css/docs.css"),
	staticFile("assets/css/examples.css"),
	staticFile("assets/css/home.css"),
	staticFile("assets/css/news.css"),
	staticFile("assets/css/workshops.css"),
	staticFile("assets/images/cloud_metrics_1.png"),
	staticFile("assets/images/cloud_metrics_2.png"),
	staticFile("assets/images/cloud_metrics_3.png"),
	staticFile("assets/images/components.svg"),
	staticFile("assets/images/discord_icon.svg"),
	staticFile("assets/images/twitter_icon.svg"),
	staticFile("assets/images/github_icon.svg"),
	staticFile("assets/images/logs_explorer.png"),
	staticFile("assets/images/trace_multi.png"),
	staticFile("assets/images/trace_single.png"),
	staticFile("assets/images/trace_gke.png"),
	staticFile("assets/images/trace_gke_local.png"),
	staticFile("assets/images/GDG_logo.png"),
	staticFile("assets/images/aicamp_logo.png"),
	staticFile("assets/js/copy.js"),
	staticFile("assets/js/placement.js"),
	staticFile("assets/js/snap.svg-min.js"),
	staticFile("assets/js/toc.js"),
	staticFile("assets/docs/hotos23_vision_paper.pdf"),
	staticFile("assets/docs/hotos23_vision_paper_slides.pdf"),
	staticFile("assets/videos/emoji_search_demo.webm"),
	staticFile("assets/videos/online_boutique_demo.webm"),
	// TODO: Add todo.js? It is currently not useful since our goldmark
	// configuration drops unknown HTML.
}

// file represents a single file to be written. If some way to generate the file
// is specified (e.g., markdown), the file contents are generated that way.
// Otherwise the file with the same name as dst is copied to dst.
type file struct {
	dst      string // Name of the destination file
	template string // Template file to use for markdown/html
	title    string // Title to pass to template.
	license  bool   // Add license to generated output.

	// Exactly one of the following should be set
	copy     string // if non-empty, file is copied to dst
	markdown string // If non-empty, file is converted to HTML and embedded in template
	html     string // If non-empty, file is read and embedded in template
}

func staticFile(name string) file {
	return file{dst: name, copy: name}
}

func main() {
	flag.Parse()

	if err := os.Chdir("website"); err != nil {
		fmt.Fprintln(os.Stderr, err, "(make sure you are in the top-level weaver directory)")
		os.Exit(1)
	}

	if err := build("public", "templates/*", files); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *watch {
		if err := watchAndBuild("public", "templates/*", files); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func watchAndBuild(dstDir, templateGlob string, files []file) error {
	// Gather the set of directories to watch. As per the fsnotify
	// documentation [1], we don't watch the files individually.
	//
	// [1]: https://pkg.go.dev/github.com/fsnotify/fsnotify#NewWatcher
	dirs := map[string]bool{}
	for _, f := range files {
		switch {
		case f.copy != "":
			dirs[filepath.Dir(f.copy)] = true
		case f.markdown != "":
			dirs[filepath.Dir(f.markdown)] = true
		case f.html != "":
			dirs[filepath.Dir(f.html)] = true
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer watcher.Close()
	for dir := range dirs {
		fmt.Printf("Watching %q\n", dir)
		if err := watcher.Add(dir); err != nil {
			return err
		}
	}

	for {
		select {
		case event := <-watcher.Events:
			if event.Op != fsnotify.Write {
				continue
			}
			fmt.Println(event)
			if err := build(dstDir, templateGlob, files); err != nil {
				return err
			}
		case err := <-watcher.Errors:
			return err
		}
	}
}

func build(dstDir, templateGlob string, files []file) error {
	t, err := template.
		New("docgen").
		Funcs(template.FuncMap{"prefix": strings.HasPrefix}).
		ParseGlob(templateGlob)
	if err != nil {
		return err
	}

	// Use the monokai style, but don't color all names and functions green.
	builder := styles.Get("monokai").Builder()
	builder.Add(chroma.NameOther, "#ffffff")
	builder.Add(chroma.NameFunction, "#ffffff")
	style, err := builder.Build()
	if err != nil {
		return err
	}

	md := goldmark.New(
		goldmark.WithExtensions(
			highlighting.NewHighlighting(
				highlighting.WithCustomStyle(style),
			),
			extension.Table,
		),
		// Render raw HTML tags.
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
	for _, f := range files {
		var err error
		switch {
		case f.copy != "":
			err = copyFile(dstDir, f.dst, f.copy)
		case f.markdown != "":
			err = buildMarkdown(t, md, dstDir, f)
		case f.html != "":
			err = buildHTML(t, md, dstDir, f)
		default:
			err = fmt.Errorf("no way to generate %s (add markdown, html, or copy field)", f.dst)
		}
		if err != nil {
			return err
		}
		if f.license {
			if err := addLicense(filepath.Join(dstDir, f.dst)); err != nil {
				return err
			}
		}
	}
	return nil
}

func buildMarkdown(t *template.Template, md goldmark.Markdown, dstDir string, f file) error {
	// Convert markdown to HTML
	input, err := os.ReadFile(f.markdown)
	if err != nil {
		return err
	}
	if len(input) == 0 {
		return fmt.Errorf("empty markdown file")
	}

	var buf bytes.Buffer
	if err := md.Convert(input, &buf); err != nil {
		return err
	}
	html := buf.Bytes()
	return executeTemplate(t, dstDir, f, template.HTML(html))
}

func buildHTML(t *template.Template, md goldmark.Markdown, dstDir string, f file) error {
	input, err := os.ReadFile(f.html)
	if err != nil {
		return err
	}
	input, err = processHTML(md, input)
	if err != nil {
		return err
	}
	return executeTemplate(t, dstDir, f, template.HTML(input))
}

// markdownRegexp finds sections of the form
//
//	<markdown>
//	text
//	<markdown>
//
// Each such section is replaced by markdown output when fed text.
var markdownRegexp = regexp.MustCompile("(?s)<markdown>\n?(.*?)</markdown>\n?")

func processHTML(md goldmark.Markdown, input []byte) ([]byte, error) {
	result := make([]byte, 0, len(input))
	processed := 0
	for _, m := range markdownRegexp.FindAllSubmatchIndex(input, -1) {
		// Add stuff before match
		result = append(result, input[processed:m[0]]...)

		// Run markdown on captured text.
		mdInput := input[m[2]:m[3]]
		var mdOutput bytes.Buffer
		if err := md.Convert(mdInput, &mdOutput); err != nil {
			return nil, err
		}
		result = append(result, []byte("<!-- begin markdown output -->\n")...)
		result = append(result, mdOutput.Bytes()...)
		result = append(result, []byte("\n<!-- end markdown output -->\n")...)

		// Advance past end of match
		processed = m[1]
	}
	result = append(result, input[processed:]...)
	return result, nil
}

func executeTemplate(t *template.Template, dstDir string, f file, contents template.HTML) error {
	type page struct {
		Title    string
		Path     string
		Contents template.HTML
	}
	d := page{
		Title:    f.title,
		Path:     f.dst,
		Contents: contents,
	}
	var output bytes.Buffer
	if err := t.ExecuteTemplate(&output, f.template, d); err != nil {
		return err
	}
	return writeFile(filepath.Join(dstDir, f.dst), output.Bytes())
}

func copyFile(dstDir, dst, src string) error {
	input, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(dstDir, dst), input)
}

func writeFile(dstName string, data []byte) error {
	dir, base := filepath.Dir(dstName), filepath.Base(dstName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	// Write to another file and rename atomically.
	tmp, err := os.CreateTemp(dir, base+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	n, err := tmp.Write(data)
	if err != nil {
		tmp.Close()
		return err
	}
	if n != len(data) {
		tmp.Close()
		return fmt.Errorf("partial write (%d of %d bytes) to %s", n, len(data), tmpName)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dstName)
}

func addLicense(fname string) error {
	return exec.Command("addlicense", "-c", "Google LLC", "-l", "apache", fname).Run()
}
