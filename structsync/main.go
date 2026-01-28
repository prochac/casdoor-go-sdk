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
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "structsync.yaml", "Path to config file")
	dryRun := flag.Bool("dry-run", false, "Preview changes without applying them")
	showDiff := flag.Bool("diff", false, "Show unified diff of changes")
	structFilter := flag.String("struct", "", "Only sync specific struct")
	markDeprecated := flag.Bool("mark-deprecated", true, "Mark removed fields as deprecated instead of deleting")
	pruneDeprecated := flag.Bool("prune-deprecated", false, "Remove previously deprecated fields")
	noColor := flag.Bool("no-color", false, "Disable colored output")
	verbose := flag.Bool("verbose", false, "Verbose output")

	var sourceOverrides flagSlice
	flag.Var(&sourceOverrides, "source-override", "Override a source with a local path (name:path), can be repeated")

	flag.Parse()

	// --diff implies --dry-run unless explicitly applying
	if *showDiff && !flag.Lookup("dry-run").Value.(flag.Getter).Get().(bool) {
		*dryRun = true
	}

	// Load configuration
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid config: %v\n", err)
		os.Exit(1)
	}

	// Parse source overrides
	overrides := make(map[string]string)
	configDir := filepath.Dir(*configPath)
	for _, ov := range sourceOverrides {
		parts := strings.SplitN(ov, ":", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "Invalid --source-override format %q, expected name:path\n", ov)
			os.Exit(1)
		}
		p := parts[1]
		if !filepath.IsAbs(p) {
			p = filepath.Join(configDir, p)
		}
		overrides[parts[0]] = p
	}

	// Clone sources (or use overrides)
	sourcePaths, cleanup, err := CloneSources(cfg.Sources, overrides)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error cloning sources: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	// Create syncer
	syncer := &Syncer{
		Config:          cfg,
		SourcePaths:     sourcePaths,
		DryRun:          *dryRun,
		ShowDiff:        *showDiff,
		StructFilter:    *structFilter,
		MarkDeprecated:  *markDeprecated,
		PruneDeprecated: *pruneDeprecated,
		NoColor:         *noColor,
		Verbose:         *verbose,
	}

	// Run sync
	if err := syncer.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// flagSlice implements flag.Value for repeatable string flags
type flagSlice []string

func (f *flagSlice) String() string { return strings.Join(*f, ", ") }
func (f *flagSlice) Set(value string) error {
	*f = append(*f, value)
	return nil
}

// Syncer orchestrates the struct synchronization
type Syncer struct {
	Config          *Config
	SourcePaths     map[string]string
	DryRun          bool
	ShowDiff        bool
	StructFilter    string
	MarkDeprecated  bool
	PruneDeprecated bool
	NoColor         bool
	Verbose         bool

	stats SummaryStats
}

// Run executes the synchronization
func (s *Syncer) Run() error {
	if s.Verbose {
		fmt.Printf("Target: %s\n", s.Config.Target)
		fmt.Printf("Sources:\n")
		for name, path := range s.SourcePaths {
			fmt.Printf("  %s: %s\n", name, path)
		}
		fmt.Printf("Structs to sync: %d\n", len(s.Config.Structs))
	}

	// Process each struct definition
	for _, structDef := range s.Config.Structs {
		// Apply filter if specified
		if s.StructFilter != "" && structDef.Name != s.StructFilter {
			continue
		}

		if err := s.syncStruct(structDef); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %s: %v\n", structDef.Name, err)
			continue
		}

		s.stats.TotalStructs++
	}

	// Print summary
	if s.DryRun || s.ShowDiff {
		fmt.Print(FormatSummary(s.stats))
	}

	return nil
}

// syncStruct synchronizes a single struct definition
func (s *Syncer) syncStruct(def StructDef) error {
	// Look up source path
	sourceDir, ok := s.SourcePaths[def.Source]
	if !ok {
		return fmt.Errorf("source %q not found in resolved paths", def.Source)
	}

	// Use SourceFile if specified, otherwise use File
	srcFile := def.File
	if def.SourceFile != "" {
		srcFile = def.SourceFile
	}
	sourceFile := filepath.Join(sourceDir, srcFile)
	targetFile := filepath.Join(s.Config.Target, def.File)

	if s.Verbose {
		fmt.Printf("\nSyncing %s from %s to %s\n", def.Name, sourceFile, targetFile)
	}

	// Parse source file
	sourceParsed, err := ParseFile(sourceFile)
	if err != nil {
		return fmt.Errorf("parsing source: %w", err)
	}

	// Extract source struct
	sourceStruct, err := ExtractStruct(sourceParsed, def.Name)
	if err != nil {
		return fmt.Errorf("extracting source struct: %w", err)
	}

	// Also extract included types if specified
	var additionalStructs []*ParsedStruct
	if len(def.IncludeTypes) > 0 {
		additional, err := ExtractStructs(sourceParsed, def.IncludeTypes)
		if err != nil {
			if s.Verbose {
				fmt.Printf("  Warning: could not extract some included types: %v\n", err)
			}
		}
		additionalStructs = additional
	}

	// Parse target file
	targetParsed, err := ParseFile(targetFile)
	if err != nil {
		return fmt.Errorf("parsing target: %w", err)
	}

	// Extract target struct
	targetStruct, err := ExtractStruct(targetParsed, def.Name)
	if err != nil {
		return fmt.Errorf("extracting target struct: %w", err)
	}

	// Transform source struct
	transformed := TransformStruct(sourceStruct, s.Config.Transform)

	if s.Verbose {
		fmt.Printf("  Source fields: %d, Excluded: %d\n",
			len(transformed.Fields),
			len(transformed.ExcludedFields))
		fmt.Printf("  Target fields: %d\n", len(targetStruct.Fields))
	}

	// Compute diff
	diff := ComputeDiff(targetStruct, transformed)

	if diff.HasChanges {
		s.stats.ChangedStructs++
	}

	// Show diff if requested
	if s.ShowDiff && diff.HasChanges {
		if s.NoColor {
			fmt.Print(FormatDiff(diff, targetFile))
		} else {
			fmt.Print(FormatColorDiff(diff, targetFile))
		}
	}

	// Apply changes if not dry-run
	if !s.DryRun && diff.HasChanges {
		// Build new fields
		mergeOpts := MergeOptions{
			MarkDeprecated:     s.MarkDeprecated,
			DeprecationMessage: "removed from server",
			PruneDeprecated:    s.PruneDeprecated,
			RemoveTags:         s.Config.Transform.RemoveTags,
		}

		newFields := BuildFieldsFromTransform(transformed, targetStruct.Fields, mergeOpts)

		// Update the struct in the target file
		if err := UpdateStructInFile(targetParsed, def.Name, newFields); err != nil {
			return fmt.Errorf("updating struct: %w", err)
		}

		// Handle additional structs
		for _, addStruct := range additionalStructs {
			addTransformed := TransformStruct(addStruct, s.Config.Transform)

			// Check if struct exists in target
			existingStruct, err := ExtractStruct(targetParsed, addStruct.Name)
			if err != nil {
				// Struct doesn't exist, would need to add it
				if s.Verbose {
					fmt.Printf("  Note: %s not found in target, skipping\n", addStruct.Name)
				}
				continue
			}

			addFields := BuildFieldsFromTransform(addTransformed, existingStruct.Fields, mergeOpts)
			if err := UpdateStructInFile(targetParsed, addStruct.Name, addFields); err != nil {
				return fmt.Errorf("updating additional struct %s: %w", addStruct.Name, err)
			}
		}

		// Write the modified file
		if err := WriteFile(targetParsed); err != nil {
			return fmt.Errorf("writing file: %w", err)
		}

		fmt.Printf("Updated %s in %s\n", def.Name, targetFile)
	} else if s.DryRun && diff.HasChanges {
		fmt.Printf("Would update %s in %s\n", def.Name, targetFile)
	} else if s.Verbose {
		fmt.Printf("  No changes needed for %s\n", def.Name)
	}

	// Update stats
	s.stats.ModifiedTags += transformed.ModifiedTags

	return nil
}
