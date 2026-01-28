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
	"strings"

	"github.com/fatih/structtag"
)

// TransformResult holds the result of transforming a struct
type TransformResult struct {
	Fields         []TransformedField
	ExcludedFields []string // Fields that were excluded entirely
	ModifiedTags   int      // Count of fields with modified tags
}

// TransformedField represents a field after transformation
type TransformedField struct {
	ParsedField
	NewTags       string // The transformed tag string
	NewType       string // The transformed type (if type mapping applied)
	ShouldExclude bool   // Whether this field should be excluded entirely
}

// TransformStruct applies transformations to a parsed struct
func TransformStruct(ps *ParsedStruct, opts TransformOpts) *TransformResult {
	result := &TransformResult{}

	// Build set of embedded types to exclude
	excludeEmbeddedSet := make(map[string]bool)
	for _, t := range opts.ExcludeEmbedded {
		excludeEmbeddedSet[t] = true
	}

	// Build set of tags to remove
	removeSet := make(map[string]bool)
	for _, t := range opts.RemoveTags {
		removeSet[t] = true
	}

	for _, field := range ps.Fields {
		tf := TransformedField{
			ParsedField: field,
			NewTags:     field.Tags,
			NewType:     field.Type, // Default to original type
		}

		// Check if this is an embedded type that should be excluded
		if field.IsEmbedded && excludeEmbeddedSet[field.Type] {
			tf.ShouldExclude = true
			result.ExcludedFields = append(result.ExcludedFields, field.Type)
			// Don't add to result.Fields - it's excluded
			continue
		}

		// Check if field should be excluded (both xorm:"-" and json:"-")
		if shouldExcludeField(field.Tags) {
			tf.ShouldExclude = true
			result.ExcludedFields = append(result.ExcludedFields, field.Name)
			// Don't add to result.Fields - it's excluded
			continue
		}

		// Apply type mappings
		if mappedType, ok := opts.TypeMappings[field.Type]; ok {
			tf.NewType = mappedType
		}

		// Transform tags (remove xorm, etc.)
		newTags, modified := transformTags(field.Tags, removeSet)
		if modified {
			tf.NewTags = newTags
			result.ModifiedTags++
		}

		result.Fields = append(result.Fields, tf)
	}

	return result
}

// shouldExcludeField checks if a field has both xorm:"-" and json:"-"
func shouldExcludeField(tagStr string) bool {
	if tagStr == "" {
		return false
	}

	// Remove backticks
	tagStr = strings.Trim(tagStr, "`")
	if tagStr == "" {
		return false
	}

	tags, err := structtag.Parse(tagStr)
	if err != nil {
		return false
	}

	xormDash := false
	jsonDash := false

	if tag, err := tags.Get("xorm"); err == nil {
		xormDash = tag.Name == "-"
	}
	if tag, err := tags.Get("json"); err == nil {
		jsonDash = tag.Name == "-"
	}

	return xormDash && jsonDash
}

// transformTags removes specified tags from a tag string
func transformTags(tagStr string, removeSet map[string]bool) (string, bool) {
	if tagStr == "" {
		return "", false
	}

	// Remove backticks
	tagStr = strings.Trim(tagStr, "`")
	if tagStr == "" {
		return "", false
	}

	tags, err := structtag.Parse(tagStr)
	if err != nil {
		return "`" + tagStr + "`", false
	}

	modified := false
	var remaining []*structtag.Tag

	for _, tag := range tags.Tags() {
		if removeSet[tag.Key] {
			modified = true
			continue
		}
		remaining = append(remaining, tag)
	}

	if !modified {
		return "`" + tagStr + "`", false
	}

	if len(remaining) == 0 {
		return "", true
	}

	// Rebuild tag string
	newTags := &structtag.Tags{}
	for _, tag := range remaining {
		newTags.Set(tag)
	}

	return "`" + newTags.String() + "`", true
}

// GetJSONFieldName extracts the JSON field name from a tag string
func GetJSONFieldName(tagStr string) string {
	if tagStr == "" {
		return ""
	}

	tagStr = strings.Trim(tagStr, "`")
	if tagStr == "" {
		return ""
	}

	tags, err := structtag.Parse(tagStr)
	if err != nil {
		return ""
	}

	tag, err := tags.Get("json")
	if err != nil {
		return ""
	}

	return tag.Name
}

// HasTag checks if a tag string contains a specific tag key
func HasTag(tagStr, key string) bool {
	if tagStr == "" {
		return false
	}

	tagStr = strings.Trim(tagStr, "`")
	if tagStr == "" {
		return false
	}

	tags, err := structtag.Parse(tagStr)
	if err != nil {
		return false
	}

	_, err = tags.Get(key)
	return err == nil
}
