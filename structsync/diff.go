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
	"bytes"
	"fmt"
	"strings"
)

// DiffResult represents the diff between two structs
type DiffResult struct {
	StructName string
	HasChanges bool
	Lines      []DiffLine
}

// DiffLine represents a single line in the diff
type DiffLine struct {
	Type    DiffType
	Content string
}

// DiffType indicates the type of diff line
type DiffType int

const (
	DiffUnchanged DiffType = iota
	DiffAdded
	DiffRemoved
	DiffModified
)

// ComputeDiff computes the diff between source and target structs
func ComputeDiff(targetStruct *ParsedStruct, transformed *TransformResult) *DiffResult {
	result := &DiffResult{
		StructName: targetStruct.Name,
	}

	// Build maps for comparison
	targetFields := make(map[string]ParsedField)
	for _, f := range targetStruct.Fields {
		key := fieldKey(f)
		targetFields[key] = f
	}

	sourceFields := make(map[string]TransformedField)
	for _, f := range transformed.Fields {
		key := tfieldKey(f)
		sourceFields[key] = f
	}

	// Add struct header
	result.Lines = append(result.Lines, DiffLine{
		Type:    DiffUnchanged,
		Content: fmt.Sprintf("type %s struct {", targetStruct.Name),
	})

	// Process source fields (the desired state)
	for _, sf := range transformed.Fields {
		key := tfieldKey(sf)

		if sf.ShouldExclude {
			// Field is excluded - don't show it
			continue
		}

		if tf, exists := targetFields[key]; exists {
			// Field exists in both
			oldLine := formatField(tf)
			newLine := formatTransformedField(sf)

			if oldLine != newLine {
				result.HasChanges = true
				result.Lines = append(result.Lines, DiffLine{
					Type:    DiffRemoved,
					Content: oldLine,
				})
				result.Lines = append(result.Lines, DiffLine{
					Type:    DiffAdded,
					Content: newLine,
				})
			} else {
				result.Lines = append(result.Lines, DiffLine{
					Type:    DiffUnchanged,
					Content: newLine,
				})
			}
		} else {
			// New field
			result.HasChanges = true
			result.Lines = append(result.Lines, DiffLine{
				Type:    DiffAdded,
				Content: formatTransformedField(sf),
			})
		}
	}

	// Check for removed fields (in target but not in source)
	for _, tf := range targetStruct.Fields {
		key := fieldKey(tf)
		if _, exists := sourceFields[key]; !exists {
			// Check if it's a commented field
			if isFieldCommented(tf) {
				continue
			}
			result.HasChanges = true
			result.Lines = append(result.Lines, DiffLine{
				Type:    DiffRemoved,
				Content: formatField(tf),
			})
		}
	}

	// Add struct footer
	result.Lines = append(result.Lines, DiffLine{
		Type:    DiffUnchanged,
		Content: "}",
	})

	return result
}

// FormatDiff formats a diff result as a unified diff string
func FormatDiff(diff *DiffResult, targetPath string) string {
	var buf bytes.Buffer

	if !diff.HasChanges {
		return ""
	}

	buf.WriteString(fmt.Sprintf("--- a/%s\n", targetPath))
	buf.WriteString(fmt.Sprintf("+++ b/%s\n", targetPath))
	buf.WriteString(fmt.Sprintf("@@ struct %s @@\n", diff.StructName))

	for _, line := range diff.Lines {
		switch line.Type {
		case DiffUnchanged:
			buf.WriteString(" " + line.Content + "\n")
		case DiffAdded:
			buf.WriteString("+" + line.Content + "\n")
		case DiffRemoved:
			buf.WriteString("-" + line.Content + "\n")
		}
	}

	return buf.String()
}

// FormatColorDiff formats a diff with ANSI colors
func FormatColorDiff(diff *DiffResult, targetPath string) string {
	var buf bytes.Buffer

	if !diff.HasChanges {
		return ""
	}

	const (
		colorReset  = "\033[0m"
		colorRed    = "\033[31m"
		colorGreen  = "\033[32m"
		colorYellow = "\033[33m"
		colorCyan   = "\033[36m"
	)

	buf.WriteString(fmt.Sprintf("%s--- a/%s%s\n", colorYellow, targetPath, colorReset))
	buf.WriteString(fmt.Sprintf("%s+++ b/%s%s\n", colorYellow, targetPath, colorReset))
	buf.WriteString(fmt.Sprintf("%s@@ struct %s @@%s\n", colorCyan, diff.StructName, colorReset))

	for _, line := range diff.Lines {
		switch line.Type {
		case DiffUnchanged:
			buf.WriteString(" " + line.Content + "\n")
		case DiffAdded:
			buf.WriteString(colorGreen + "+" + line.Content + colorReset + "\n")
		case DiffRemoved:
			buf.WriteString(colorRed + "-" + line.Content + colorReset + "\n")
		}
	}

	return buf.String()
}

// fieldKey returns a unique key for a parsed field
func fieldKey(f ParsedField) string {
	if f.IsEmbedded {
		return "embedded:" + f.Type
	}
	return "field:" + f.Name
}

// tfieldKey returns a unique key for a transformed field
func tfieldKey(f TransformedField) string {
	if f.IsEmbedded {
		return "embedded:" + f.Type
	}
	return "field:" + f.Name
}

// formatField formats a ParsedField as a Go struct field line
func formatField(f ParsedField) string {
	var buf bytes.Buffer
	buf.WriteString("\t")

	if f.IsEmbedded {
		buf.WriteString(f.Type)
	} else {
		buf.WriteString(f.Name)
		buf.WriteString(" ")
		buf.WriteString(f.Type)
	}

	if f.Tags != "" {
		buf.WriteString(" ")
		buf.WriteString(f.Tags)
	}

	return buf.String()
}

// formatTransformedField formats a TransformedField as a Go struct field line
func formatTransformedField(f TransformedField) string {
	var buf bytes.Buffer
	buf.WriteString("\t")

	// Use NewType if set, otherwise fall back to original Type
	fieldType := f.Type
	if f.NewType != "" {
		fieldType = f.NewType
	}

	if f.IsEmbedded {
		buf.WriteString(fieldType)
	} else {
		buf.WriteString(f.Name)
		buf.WriteString(" ")
		buf.WriteString(fieldType)
	}

	if f.NewTags != "" {
		buf.WriteString(" ")
		buf.WriteString(f.NewTags)
	}

	return buf.String()
}

// isFieldCommented checks if a field is already commented out
func isFieldCommented(f ParsedField) bool {
	for _, c := range f.Comments {
		c = strings.TrimSpace(c)
		if strings.HasPrefix(c, "//") {
			// Check if the comment contains a type-like pattern
			content := strings.TrimPrefix(c, "//")
			content = strings.TrimSpace(content)
			if strings.HasPrefix(content, "*") || strings.Contains(content, ".") {
				return true
			}
		}
	}
	return false
}

// SummaryStats holds summary statistics for the sync operation
type SummaryStats struct {
	TotalStructs   int
	ChangedStructs int
	NewFields      int
	RemovedFields  int
	ModifiedTags   int
	ExcludedFields int
}

// FormatSummary formats summary statistics as a string
func FormatSummary(stats SummaryStats) string {
	var buf bytes.Buffer

	buf.WriteString("\n=== Sync Summary ===\n")
	buf.WriteString(fmt.Sprintf("Structs processed: %d\n", stats.TotalStructs))
	buf.WriteString(fmt.Sprintf("Structs changed:   %d\n", stats.ChangedStructs))
	buf.WriteString(fmt.Sprintf("New fields:        %d\n", stats.NewFields))
	buf.WriteString(fmt.Sprintf("Removed fields:    %d\n", stats.RemovedFields))
	buf.WriteString(fmt.Sprintf("Modified tags:     %d\n", stats.ModifiedTags))
	buf.WriteString(fmt.Sprintf("Excluded fields:   %d\n", stats.ExcludedFields))

	return buf.String()
}
