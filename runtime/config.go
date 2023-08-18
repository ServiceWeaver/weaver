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

package runtime

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/ServiceWeaver/weaver/internal/env"
	"github.com/ServiceWeaver/weaver/runtime/protos"
)

// ParseConfig parses the specified configuration input, which should
// hold a set of sections in TOML format from the specified file.
// The section corresponding to the common Service Weaver application
// configuration is parsed and returned as a *AppConfig.
//
// sectionValidator(key, val) is used to validate every section config entry.
func ParseConfig(file string, input string, sectionValidator func(string, string) error) (*protos.AppConfig, error) {
	// Extract sections from toml file.
	var sections map[string]toml.Primitive
	_, err := toml.Decode(input, &sections)
	if err != nil {
		return nil, err
	}
	config := &protos.AppConfig{Sections: map[string]string{}}
	for k, v := range sections {
		var buf strings.Builder
		err := toml.NewEncoder(&buf).Encode(v)
		if err != nil {
			return nil, fmt.Errorf("encoding section %q: %w", k, err)
		}
		config.Sections[k] = buf.String()
	}

	// Parse app section.
	if err := extractApp(file, config); err != nil {
		return nil, err
	}

	for key, val := range config.Sections {
		if err := sectionValidator(key, val); err != nil {
			return nil, err
		}
	}

	return config, nil
}

// ParseConfigSection parses the config section for key into dst.
// If shortKey is not empty, either key or shortKey is accepted.
// If the named section is not found, returns nil without changing dst.
func ParseConfigSection(key, shortKey string, sections map[string]string, dst any) error {
	section, ok := sections[key]
	if shortKey != "" {
		// Fetch section listed for shortKey, if any.
		if shortKeySection, ok2 := sections[shortKey]; ok2 {
			if ok {
				return fmt.Errorf("conflicting sections %q and %q", shortKey, key)
			}
			key, section, ok = shortKey, shortKeySection, ok2
		}
	}
	if !ok { // not found
		return nil
	}

	// Parse and validate the section.
	md, err := toml.Decode(section, dst)
	if err != nil {
		return err
	}
	if unknown := md.Undecoded(); len(unknown) != 0 {
		return fmt.Errorf("section %q has unknown keys %v", key, unknown)
	}
	if x, ok := dst.(interface{ Validate() error }); ok {
		if err := x.Validate(); err != nil {
			return fmt.Errorf("section %q: %w", key, err)
		}
	}
	return nil
}

func extractApp(file string, config *protos.AppConfig) error {
	const appKey = "github.com/ServiceWeaver/weaver"
	const shortAppKey = "serviceweaver"

	// appConfig holds the data from under appKey in the TOML config.
	// It matches the contents of the Config proto.
	type appConfig struct {
		Name     string
		Binary   string
		Args     []string
		Env      []string
		Colocate [][]string
		Rollout  time.Duration
	}

	parsed := &appConfig{}
	if err := ParseConfigSection(appKey, shortAppKey, config.Sections, parsed); err != nil {
		return err
	}

	// Move struct fields into proto.
	config.Name = parsed.Name
	config.Binary = parsed.Binary
	config.Args = parsed.Args
	config.Env = parsed.Env
	config.RolloutNanos = int64(parsed.Rollout)
	for _, colocate := range parsed.Colocate {
		group := &protos.ComponentGroup{Components: colocate}
		config.Colocate = append(config.Colocate, group)
	}

	// Canonicalize the config.
	if err := canonicalizeConfig(config, filepath.Dir(file)); err != nil {
		return err
	}
	return nil
}

// canonicalizeConfig updates the provided config to canonical
// form. All relative paths inside the configuration are resolved
// relative to the provided directory.
func canonicalizeConfig(c *protos.AppConfig, dir string) error {
	// Fill in the application name if necessary.
	bin := c.GetBinary()
	if c.Name == "" && bin != "" {
		c.Name = filepath.Base(bin)
	}

	// Convert relative paths inside the application config to absolute paths
	// interpreted starting at the directory containing the config file.
	if !filepath.IsAbs(bin) {
		bin, err := filepath.Abs(filepath.Join(dir, bin))
		if err != nil {
			return err
		}
		c.Binary = bin
	}

	// Validate the environment variables.
	if _, err := env.Parse(c.Env); err != nil {
		return fmt.Errorf("invalid Env: %v", err)
	}

	// Validate the same_process entry.
	if err := checkSameProcess(c); err != nil {
		return err
	}
	return nil
}

// checkSameProcess checks that the same_process entry is valid.
func checkSameProcess(c *protos.AppConfig) error {
	seen := map[string]struct{}{}
	for _, components := range c.Colocate {
		for _, component := range components.Components {
			if _, ok := seen[component]; ok {
				return fmt.Errorf("component %q placed multiple times", component)
			}
			seen[component] = struct{}{}
		}
	}
	return nil
}
