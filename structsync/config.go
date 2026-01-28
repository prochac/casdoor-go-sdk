// Copyright 2024 The Casdoor Authors. All Rights Reserved.
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
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config represents the structsync configuration
type Config struct {
	Sources     map[string]SourceDef `yaml:"sources"`
	Target      string               `yaml:"target"`
	Structs     []StructDef          `yaml:"structs"`
	Transform   TransformOpts        `yaml:"transform"`
	Deprecation DeprecationOpt       `yaml:"deprecation"`
}

// SourceDef defines a git repo source
type SourceDef struct {
	Repo string `yaml:"repo"`
	Path string `yaml:"path,omitempty"`
	Ref  string `yaml:"ref,omitempty"`
}

// StructDef defines a struct to sync
type StructDef struct {
	Name         string   `yaml:"name"`
	Source       string   `yaml:"source"`                // Key into Sources map
	File         string   `yaml:"file"`                  // Target file in SDK
	SourceFile   string   `yaml:"source_file,omitempty"` // Source file (if different from File)
	IncludeTypes []string `yaml:"include_types,omitempty"`
}

// TransformOpts defines transformation options
type TransformOpts struct {
	RemoveTags      []string          `yaml:"remove_tags"`
	ExcludeEmbedded []string          `yaml:"exclude_embedded"`
	TypeMappings    map[string]string `yaml:"type_mappings"` // Map external types to SDK types, e.g., "pp.PaymentState": "string"
}

// DeprecationOpt defines deprecation handling options
type DeprecationOpt struct {
	MarkRemoved    bool `yaml:"mark_removed"`
	IncludeCommit  bool `yaml:"include_commit"`
	AutoPruneAfter int  `yaml:"auto_prune_after"`
}

// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	// Resolve target path relative to config file location
	configDir := filepath.Dir(path)
	if !filepath.IsAbs(cfg.Target) {
		cfg.Target = filepath.Join(configDir, cfg.Target)
	}

	return &cfg, nil
}

// Validate checks that the configuration is valid
func (c *Config) Validate() error {
	if len(c.Sources) == 0 {
		return fmt.Errorf("no sources defined")
	}
	if c.Target == "" {
		return fmt.Errorf("target directory not specified")
	}
	if len(c.Structs) == 0 {
		return fmt.Errorf("no structs defined")
	}

	// Check all struct Source refs exist in Sources map
	for _, s := range c.Structs {
		if s.Source == "" {
			return fmt.Errorf("struct %s: source not specified", s.Name)
		}
		if _, ok := c.Sources[s.Source]; !ok {
			return fmt.Errorf("struct %s: source %q not found in sources", s.Name, s.Source)
		}
	}

	// Check target directory exists
	if _, err := os.Stat(c.Target); os.IsNotExist(err) {
		return fmt.Errorf("target directory does not exist: %s", c.Target)
	}

	return nil
}
