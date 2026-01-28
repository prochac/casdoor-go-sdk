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
	"os/exec"
	"path/filepath"
)

// CloneSources clones all source repos to a temporary directory.
// Returns a map of source name → local path (clone dir + subpath) and a cleanup function.
func CloneSources(sources map[string]SourceDef, overrides map[string]string) (map[string]string, func(), error) {
	paths := make(map[string]string)
	noop := func() {}

	// Apply overrides first — these don't need cloning
	for name, localPath := range overrides {
		if _, ok := sources[name]; !ok {
			return nil, noop, fmt.Errorf("source override %q not found in sources config", name)
		}
		paths[name] = localPath
	}

	// Determine which sources need cloning
	var toClone []string
	for name := range sources {
		if _, overridden := overrides[name]; !overridden {
			toClone = append(toClone, name)
		}
	}

	if len(toClone) == 0 {
		return paths, noop, nil
	}

	// Create single temp dir for all clones
	tmpDir, err := os.MkdirTemp("", "structsync-*")
	if err != nil {
		return nil, noop, fmt.Errorf("creating temp dir: %w", err)
	}
	cleanup := func() { os.RemoveAll(tmpDir) }

	for _, name := range toClone {
		src := sources[name]
		cloneDir := filepath.Join(tmpDir, name)

		args := []string{"clone", "--depth", "1"}
		if src.Ref != "" {
			args = append(args, "--branch", src.Ref)
		}
		args = append(args, src.Repo, cloneDir)

		fmt.Printf("Cloning %s from %s...\n", name, src.Repo)
		cmd := exec.Command("git", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			cleanup()
			return nil, noop, fmt.Errorf("cloning %s: %w", name, err)
		}

		localPath := cloneDir
		if src.Path != "" {
			localPath = filepath.Join(cloneDir, src.Path)
		}
		paths[name] = localPath
	}

	return paths, cleanup, nil
}
