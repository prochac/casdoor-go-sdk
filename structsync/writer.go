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
	"go/token"
	"os"
	"strings"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
)

// WriteFile writes a modified DST file back to disk
func WriteFile(pf *ParsedFile) error {
	var buf bytes.Buffer

	restorer := decorator.NewRestorer()
	if err := restorer.Fprint(&buf, pf.File); err != nil {
		return fmt.Errorf("formatting file: %w", err)
	}

	if err := os.WriteFile(pf.Path, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("writing file %s: %w", pf.Path, err)
	}

	return nil
}

// WriteFileToString returns the formatted file content as a string
func WriteFileToString(pf *ParsedFile) (string, error) {
	var buf bytes.Buffer

	restorer := decorator.NewRestorer()
	if err := restorer.Fprint(&buf, pf.File); err != nil {
		return "", fmt.Errorf("formatting file: %w", err)
	}

	return buf.String(), nil
}

// UpdateStructInFile updates a specific struct in a file with new fields
func UpdateStructInFile(pf *ParsedFile, structName string, buildResult *BuildResult) error {
	for _, decl := range pf.File.Decls {
		genDecl, ok := decl.(*dst.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*dst.TypeSpec)
			if !ok || typeSpec.Name.Name != structName {
				continue
			}

			structType, ok := typeSpec.Type.(*dst.StructType)
			if !ok {
				return fmt.Errorf("%s is not a struct type", structName)
			}

			structType.Fields.List = buildResult.Fields
			return nil
		}
	}

	return fmt.Errorf("struct %s not found in file", structName)
}

// BuildResult contains the result of building fields
type BuildResult struct {
	Fields []*dst.Field
}

// BuildFieldsFromTransform builds dst.Field slice from transformed fields
func BuildFieldsFromTransform(transformed *TransformResult, existingFields []ParsedField, opts MergeOptions) *BuildResult {
	result := &BuildResult{}

	// Build map of existing fields for reference
	existingMap := make(map[string]ParsedField)
	for _, f := range existingFields {
		key := f.Name
		if f.IsEmbedded {
			key = f.Type
		}
		existingMap[key] = f
	}

	// Build map of transformed fields
	transformedMap := make(map[string]TransformedField)
	for _, f := range transformed.Fields {
		key := f.Name
		if f.IsEmbedded {
			key = f.Type
		}
		transformedMap[key] = f
	}

	// Add transformed fields in order
	for _, tf := range transformed.Fields {
		if tf.ShouldExclude {
			continue
		}

		field := buildField(tf)
		result.Fields = append(result.Fields, field)
	}

	// Handle fields that exist in target but not in source
	for _, ef := range existingFields {
		key := ef.Name
		if ef.IsEmbedded {
			key = ef.Type
		}

		if _, exists := transformedMap[key]; !exists {
			if opts.MarkDeprecated {
				field := buildDeprecatedField(ef, opts.DeprecationMessage, opts.RemoveTags)
				result.Fields = append(result.Fields, field)
			}
			// If not marking deprecated, the field is simply not included (removed)
		}
	}

	return result
}

// buildField creates a dst.Field from a TransformedField
func buildField(tf TransformedField) *dst.Field {
	field := &dst.Field{}

	// Set type - use NewType if it was mapped, otherwise clone original
	if tf.NewType != "" && tf.NewType != tf.Type {
		// Type was remapped, create new type expression
		field.Type = &dst.Ident{Name: tf.NewType}
	} else {
		field.Type = cloneExpr(tf.TypeExpr)
	}

	// Set names (empty for embedded)
	if !tf.IsEmbedded {
		field.Names = []*dst.Ident{{Name: tf.Name}}
	}

	// Set tag
	if tf.NewTags != "" {
		field.Tag = &dst.BasicLit{
			Kind:  token.STRING,
			Value: tf.NewTags,
		}
	}

	// Preserve comments
	if len(tf.Comments) > 0 {
		field.Decs.Start = tf.Comments
	}

	return field
}

// preserveCommentedField preserves an existing commented field
func preserveCommentedField(pf ParsedField) *dst.Field {
	field := &dst.Field{}

	if !pf.IsEmbedded && pf.Name != "" {
		field.Names = []*dst.Ident{{Name: pf.Name}}
	}

	if pf.TypeExpr != nil {
		field.Type = cloneExpr(pf.TypeExpr)
	} else {
		field.Type = &dst.Ident{Name: pf.Type}
	}

	if pf.Tags != "" {
		field.Tag = &dst.BasicLit{
			Kind:  token.STRING,
			Value: pf.Tags,
		}
	}

	if len(pf.Comments) > 0 {
		field.Decs.Start = pf.Comments
	}

	return field
}

// buildDeprecatedField creates a field with a deprecation comment
func buildDeprecatedField(pf ParsedField, message string, removeTagKeys []string) *dst.Field {
	field := &dst.Field{}

	if !pf.IsEmbedded {
		field.Names = []*dst.Ident{{Name: pf.Name}}
	}

	if pf.TypeExpr != nil {
		field.Type = cloneExpr(pf.TypeExpr)
	} else {
		field.Type = &dst.Ident{Name: pf.Type}
	}

	// Remove specified tags from deprecated fields too
	if pf.Tags != "" {
		removeSet := make(map[string]bool)
		for _, key := range removeTagKeys {
			removeSet[key] = true
		}
		newTags, _ := transformTagsHelper(pf.Tags, removeSet)
		if newTags != "" {
			field.Tag = &dst.BasicLit{
				Kind:  token.STRING,
				Value: newTags,
			}
		}
	}

	// Add deprecation comment above the field
	if message == "" {
		message = "removed from server"
	}
	deprecation := "// Deprecated: " + message

	// Use Before=NewLine to ensure comment appears on its own line
	field.Decs.Before = dst.NewLine
	field.Decs.Start = []string{deprecation}

	return field
}

// transformTagsHelper removes specified tags from a tag string (helper for writer)
func transformTagsHelper(tagStr string, removeSet map[string]bool) (string, bool) {
	if tagStr == "" {
		return "", false
	}

	// Import the structtag package functionality
	tagStr = strings.Trim(tagStr, "`")
	if tagStr == "" {
		return "", false
	}

	// Parse the tag manually since we can't import structtag here due to package issues
	// Simple implementation: find and remove xorm:"..." pattern
	result := tagStr
	for key := range removeSet {
		// Remove pattern like: key:"value"
		for {
			start := strings.Index(result, key+":\"")
			if start == -1 {
				break
			}
			// Find the end of this tag value
			end := start + len(key) + 2 // skip key:"
			depth := 1
			for end < len(result) && depth > 0 {
				if result[end] == '"' && (end == 0 || result[end-1] != '\\') {
					depth--
				}
				end++
			}
			// Remove the tag and any trailing space
			prefix := strings.TrimRight(result[:start], " ")
			suffix := strings.TrimLeft(result[end:], " ")
			result = prefix
			if result != "" && suffix != "" {
				result += " "
			}
			result += suffix
		}
	}

	result = strings.TrimSpace(result)
	if result == "" {
		return "", true
	}

	return "`" + result + "`", true
}
