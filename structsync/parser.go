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
	"go/token"
	"os"
	"strings"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
)

// ParsedFile represents a parsed Go file with its AST
type ParsedFile struct {
	Path    string
	File    *dst.File
	Fset    *token.FileSet
	Content []byte
}

// ParsedStruct represents an extracted struct definition
type ParsedStruct struct {
	Name     string
	Fields   []ParsedField
	Comments []string
	Node     *dst.TypeSpec
	GenDecl  *dst.GenDecl
}

// ParsedField represents a struct field
type ParsedField struct {
	Name       string
	Type       string
	TypeExpr   dst.Expr
	Tags       string
	Comments   []string
	IsEmbedded bool
	Node       *dst.Field
}

// ParseFile parses a Go file and returns the DST representation
func ParseFile(path string) (*ParsedFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file %s: %w", path, err)
	}

	fset := token.NewFileSet()
	file, err := decorator.ParseFile(fset, path, content, 0)
	if err != nil {
		return nil, fmt.Errorf("parsing file %s: %w", path, err)
	}

	return &ParsedFile{
		Path:    path,
		File:    file,
		Fset:    fset,
		Content: content,
	}, nil
}

// ExtractStruct extracts a struct definition by name from a parsed file
func ExtractStruct(pf *ParsedFile, name string) (*ParsedStruct, error) {
	for _, decl := range pf.File.Decls {
		genDecl, ok := decl.(*dst.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*dst.TypeSpec)
			if !ok {
				continue
			}

			if typeSpec.Name.Name != name {
				continue
			}

			structType, ok := typeSpec.Type.(*dst.StructType)
			if !ok {
				return nil, fmt.Errorf("%s is not a struct type", name)
			}

			ps := &ParsedStruct{
				Name:    name,
				Node:    typeSpec,
				GenDecl: genDecl,
			}

			// Extract comments from the GenDecl
			if genDecl.Decs.Start != nil {
				ps.Comments = genDecl.Decs.Start.All()
			}

			// Extract fields
			for _, field := range structType.Fields.List {
				pf := extractField(field)
				ps.Fields = append(ps.Fields, pf...)
			}

			return ps, nil
		}
	}

	return nil, fmt.Errorf("struct %s not found in %s", name, pf.Path)
}

// ExtractStructs extracts multiple structs by name from a parsed file
func ExtractStructs(pf *ParsedFile, names []string) ([]*ParsedStruct, error) {
	nameSet := make(map[string]bool)
	for _, name := range names {
		nameSet[name] = true
	}

	var result []*ParsedStruct

	for _, decl := range pf.File.Decls {
		genDecl, ok := decl.(*dst.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*dst.TypeSpec)
			if !ok {
				continue
			}

			if !nameSet[typeSpec.Name.Name] {
				continue
			}

			structType, ok := typeSpec.Type.(*dst.StructType)
			if !ok {
				continue
			}

			ps := &ParsedStruct{
				Name:    typeSpec.Name.Name,
				Node:    typeSpec,
				GenDecl: genDecl,
			}

			// Extract comments from the GenDecl
			if genDecl.Decs.Start != nil {
				ps.Comments = genDecl.Decs.Start.All()
			}

			// Extract fields
			for _, field := range structType.Fields.List {
				pfs := extractField(field)
				ps.Fields = append(ps.Fields, pfs...)
			}

			result = append(result, ps)
		}
	}

	return result, nil
}

// extractField extracts field information from a dst.Field
func extractField(field *dst.Field) []ParsedField {
	var result []ParsedField

	// Get tag value
	tagValue := ""
	if field.Tag != nil {
		tagValue = field.Tag.Value
	}

	// Get comments
	var comments []string
	if field.Decs.Start != nil {
		comments = field.Decs.Start.All()
	}

	// Get type as string
	typeStr := exprToString(field.Type)

	// Check if embedded (no names)
	if len(field.Names) == 0 {
		return []ParsedField{{
			Name:       typeStr,
			Type:       typeStr,
			TypeExpr:   field.Type,
			Tags:       tagValue,
			Comments:   comments,
			IsEmbedded: true,
			Node:       field,
		}}
	}

	// Named field(s)
	for _, name := range field.Names {
		result = append(result, ParsedField{
			Name:       name.Name,
			Type:       typeStr,
			TypeExpr:   field.Type,
			Tags:       tagValue,
			Comments:   comments,
			IsEmbedded: false,
			Node:       field,
		})
	}

	return result
}

// exprToString converts a dst.Expr to its string representation
func exprToString(expr dst.Expr) string {
	switch e := expr.(type) {
	case *dst.Ident:
		return e.Name
	case *dst.StarExpr:
		return "*" + exprToString(e.X)
	case *dst.SelectorExpr:
		return exprToString(e.X) + "." + e.Sel.Name
	case *dst.ArrayType:
		if e.Len == nil {
			return "[]" + exprToString(e.Elt)
		}
		return "[...]" + exprToString(e.Elt)
	case *dst.MapType:
		return "map[" + exprToString(e.Key) + "]" + exprToString(e.Value)
	case *dst.InterfaceType:
		return "interface{}"
	case *dst.StructType:
		return "struct{}"
	default:
		return fmt.Sprintf("%T", expr)
	}
}

// FindAllStructs returns all struct type names in a file
func FindAllStructs(pf *ParsedFile) []string {
	var names []string

	for _, decl := range pf.File.Decls {
		genDecl, ok := decl.(*dst.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*dst.TypeSpec)
			if !ok {
				continue
			}

			if _, ok := typeSpec.Type.(*dst.StructType); ok {
				names = append(names, typeSpec.Name.Name)
			}
		}
	}

	return names
}

// IsCommentedField checks if a line represents a commented-out field
func IsCommentedField(comment string) (fieldType string, ok bool) {
	comment = strings.TrimSpace(comment)
	if !strings.HasPrefix(comment, "//") {
		return "", false
	}

	// Remove comment prefix
	content := strings.TrimSpace(strings.TrimPrefix(comment, "//"))

	// Check if it looks like a type (starts with * or lowercase letter for package)
	if strings.HasPrefix(content, "*") || (len(content) > 0 && content[0] >= 'a' && content[0] <= 'z') {
		// Check for common embedded type patterns
		if strings.Contains(content, ".") || strings.HasPrefix(content, "*") {
			return content, true
		}
	}

	return "", false
}
