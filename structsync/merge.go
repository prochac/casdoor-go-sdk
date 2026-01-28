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
	"go/token"
	"strings"

	"github.com/dave/dst"
)

// MergeResult contains the result of merging structs
type MergeResult struct {
	NewFields        []string // Fields added from source
	RemovedFields    []string // Fields in target but not in source (to be deprecated)
	ModifiedFields   []string // Fields with changed types or tags
	UnchangedFields  []string // Fields that didn't change
	DeprecatedFields []string // Fields marked as deprecated
}

// MergeStructs merges the transformed source struct into the target file
func MergeStructs(targetFile *ParsedFile, targetStruct *ParsedStruct, transformed *TransformResult, opts MergeOptions) (*MergeResult, error) {
	result := &MergeResult{}

	// Build map of existing target fields
	targetFields := make(map[string]ParsedField)
	for _, f := range targetStruct.Fields {
		key := f.Name
		if f.IsEmbedded {
			key = f.Type
		}
		targetFields[key] = f
	}

	// Build map of source fields
	sourceFields := make(map[string]TransformedField)
	for _, f := range transformed.Fields {
		key := f.Name
		if f.IsEmbedded {
			key = f.Type
		}
		sourceFields[key] = f
	}

	// Find the struct in the target file and replace its fields
	for _, decl := range targetFile.File.Decls {
		genDecl, ok := decl.(*dst.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*dst.TypeSpec)
			if !ok || typeSpec.Name.Name != targetStruct.Name {
				continue
			}

			structType, ok := typeSpec.Type.(*dst.StructType)
			if !ok {
				continue
			}

			// Build new field list
			var newFields []*dst.Field

			// First, add all transformed source fields in order
			for _, tf := range transformed.Fields {
				key := tf.Name
				if tf.IsEmbedded {
					key = tf.Type
				}

				// Skip excluded fields
				if tf.ShouldExclude {
					continue
				}

				// Check if field exists in target
				if existing, exists := targetFields[key]; exists {
					// Check if modified
					if fieldChanged(existing, tf) {
						result.ModifiedFields = append(result.ModifiedFields, key)
					} else {
						result.UnchangedFields = append(result.UnchangedFields, key)
					}
				} else {
					result.NewFields = append(result.NewFields, key)
				}

				field := createField(tf)
				newFields = append(newFields, field)
			}

			// Check for fields in target but not in source (deprecated)
			for key, tf := range targetFields {
				if _, exists := sourceFields[key]; !exists {
					if opts.MarkDeprecated {
						// Add deprecation comment
						result.DeprecatedFields = append(result.DeprecatedFields, key)
						field := createDeprecatedField(tf, opts.DeprecationMessage)
						newFields = append(newFields, field)
					} else {
						result.RemovedFields = append(result.RemovedFields, key)
					}
				}
			}

			// Replace the struct's fields
			structType.Fields.List = newFields

			return result, nil
		}
	}

	return result, nil
}

// MergeOptions controls merge behavior
type MergeOptions struct {
	MarkDeprecated     bool
	DeprecationMessage string
	PruneDeprecated    bool
	RemoveTags         []string // Tags to remove from deprecated fields
}

// createField creates a dst.Field from a TransformedField
func createField(tf TransformedField) *dst.Field {
	field := &dst.Field{}

	// Set names (empty for embedded fields)
	if !tf.IsEmbedded {
		field.Names = []*dst.Ident{{Name: tf.Name}}
	}

	// Clone the type expression
	field.Type = cloneExpr(tf.TypeExpr)

	// Set tag
	if tf.NewTags != "" {
		field.Tag = &dst.BasicLit{
			Kind:  token.STRING,
			Value: tf.NewTags,
		}
	}

	// Set comments if any
	if len(tf.Comments) > 0 {
		field.Decs.Start = tf.Comments
	}

	return field
}

// createDeprecatedField creates a field with deprecation comment
func createDeprecatedField(pf ParsedField, message string) *dst.Field {
	field := &dst.Field{}

	if !pf.IsEmbedded {
		field.Names = []*dst.Ident{{Name: pf.Name}}
	}

	field.Type = cloneExpr(pf.TypeExpr)

	if pf.Tags != "" {
		field.Tag = &dst.BasicLit{
			Kind:  token.STRING,
			Value: pf.Tags,
		}
	}

	// Add deprecation comment
	deprecationComment := "// Deprecated: " + message
	field.Decs.Start = append(pf.Comments, deprecationComment)

	return field
}

// fieldChanged checks if a field has changed between source and target
func fieldChanged(target ParsedField, source TransformedField) bool {
	// Check type
	if target.Type != source.Type {
		return true
	}

	// Check tags (compare the transformed tag with target)
	targetTags := strings.Trim(target.Tags, "`")
	sourceTags := strings.Trim(source.NewTags, "`")

	if targetTags != sourceTags {
		return true
	}

	return false
}

// cloneExpr creates a deep clone of a dst.Expr
func cloneExpr(expr dst.Expr) dst.Expr {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *dst.Ident:
		return &dst.Ident{Name: e.Name}
	case *dst.StarExpr:
		return &dst.StarExpr{X: cloneExpr(e.X)}
	case *dst.SelectorExpr:
		return &dst.SelectorExpr{
			X:   cloneExpr(e.X),
			Sel: &dst.Ident{Name: e.Sel.Name},
		}
	case *dst.ArrayType:
		return &dst.ArrayType{
			Len: e.Len, // Length is usually nil for slices
			Elt: cloneExpr(e.Elt),
		}
	case *dst.MapType:
		return &dst.MapType{
			Key:   cloneExpr(e.Key),
			Value: cloneExpr(e.Value),
		}
	case *dst.InterfaceType:
		return &dst.InterfaceType{
			Methods: &dst.FieldList{},
		}
	case *dst.StructType:
		return &dst.StructType{
			Fields: &dst.FieldList{},
		}
	default:
		// For complex types, return as-is (might need enhancement)
		return expr
	}
}
