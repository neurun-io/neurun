package boundary

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestActiveServerDoesNotImportLegacyOrSDKImplementations(t *testing.T) {
	t.Parallel()

	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))

	forbidden := []string{
		"github.com/dagflows/builder",
		"github.com/dagflows/worker",
		"github.com/dagflows/neurun-go",
		"github.com/dagflows/neurun-io/legacy",
	}

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".tmp", ".tmp-go-cache", "legacy", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" {
			return nil
		}
		parsed, err := parser.ParseFile(
			token.NewFileSet(),
			path,
			nil,
			parser.ImportsOnly,
		)
		if err != nil {
			return err
		}
		for _, imported := range parsed.Imports {
			importPath, err := strconv.Unquote(imported.Path.Value)
			if err != nil {
				return err
			}
			for _, forbiddenRoot := range forbidden {
				if importPath == forbiddenRoot ||
					strings.HasPrefix(importPath, forbiddenRoot+"/") {
					t.Errorf(
						"%s imports forbidden implementation boundary %s",
						path,
						importPath,
					)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
