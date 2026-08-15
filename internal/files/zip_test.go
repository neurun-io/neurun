package files

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type zipFixture struct {
	name string
	body string
	mode fs.FileMode
}

func TestExtractZIPValidArchive(t *testing.T) {
	t.Parallel()

	archive := buildZIP(t, []zipFixture{
		{name: "bin/run.sh", body: "#!/bin/sh\n", mode: 0o755},
		{name: "docs/", mode: os.ModeDir | 0o755},
		{name: "docs/readme.txt", body: "hello", mode: 0o644},
	})
	destination := filepath.Join(t.TempDir(), "expanded")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}

	stats, err := ExtractZIP(bytes.NewReader(archive), int64(len(archive)), destination, ArchiveLimits{
		MaxEntries:       10,
		MaxExpandedBytes: 1_048_576,
	})
	if err != nil {
		t.Fatalf("ExtractZIP() error = %v", err)
	}
	if stats.Files != 2 || stats.Directories != 1 || stats.ExpandedBytes != 15 {
		t.Fatalf("stats = %+v", stats)
	}
	assertFileContents(t, filepath.Join(destination, "bin", "run.sh"), "#!/bin/sh\n")
	assertFileContents(t, filepath.Join(destination, "docs", "readme.txt"), "hello")
}

func TestExtractZIPRejectsTraversalAbsoluteAndAmbiguousNames(t *testing.T) {
	t.Parallel()

	names := []string{
		"../outside.txt",
		"a/../../outside.txt",
		"/absolute.txt",
		"C:/windows.txt",
		`a\..\outside.txt`,
		"a/./file.txt",
		"a//file.txt",
		"a/\x00/file.txt",
		"a/\n/file.txt",
		"CON",
		"nul.txt",
		"trailing.",
		"trailing ",
	}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			archive := buildZIP(t, []zipFixture{{name: name, body: "bad", mode: 0o644}})
			root := t.TempDir()
			destination := filepath.Join(root, "expanded")
			_, err := ExtractZIP(bytes.NewReader(archive), int64(len(archive)), destination, ArchiveLimits{})
			if !errors.Is(err, ErrUnsafeArchive) {
				t.Fatalf("ExtractZIP() error = %v, want ErrUnsafeArchive", err)
			}
			if _, statErr := os.Stat(destination); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("destination exists after rejection: %v", statErr)
			}
			if _, statErr := os.Stat(filepath.Join(root, "outside.txt")); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("outside file exists: %v", statErr)
			}
		})
	}
}

func TestExtractZIPRejectsLinksAndSpecialFiles(t *testing.T) {
	t.Parallel()

	tests := []zipFixture{
		{name: "link", body: "../outside", mode: os.ModeSymlink | 0o777},
		{name: "pipe", mode: os.ModeNamedPipe | 0o600},
		{name: "device", mode: os.ModeDevice | 0o600},
	}
	for _, fixture := range tests {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			archive := buildZIP(t, []zipFixture{fixture})
			_, err := ExtractZIP(
				bytes.NewReader(archive),
				int64(len(archive)),
				filepath.Join(t.TempDir(), "expanded"),
				ArchiveLimits{},
			)
			if !errors.Is(err, ErrUnsafeArchive) {
				t.Fatalf("ExtractZIP() error = %v, want ErrUnsafeArchive", err)
			}
		})
	}
}

func TestExtractZIPRejectsEntryAndExpandedByteLimit(t *testing.T) {
	t.Parallel()

	archive := buildZIP(t, []zipFixture{
		{name: "one", body: "1", mode: 0o600},
		{name: "two", body: "22", mode: 0o600},
	})

	_, err := ExtractZIP(
		bytes.NewReader(archive),
		int64(len(archive)),
		filepath.Join(t.TempDir(), "entries"),
		ArchiveLimits{MaxEntries: 1, MaxExpandedBytes: 100},
	)
	if !errors.Is(err, ErrTooManyArchiveFiles) {
		t.Fatalf("entry limit error = %v", err)
	}

	destination := filepath.Join(t.TempDir(), "bytes")
	_, err = ExtractZIP(
		bytes.NewReader(archive),
		int64(len(archive)),
		destination,
		ArchiveLimits{MaxEntries: 10, MaxExpandedBytes: 2},
	)
	if !errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("byte limit error = %v", err)
	}
	if _, statErr := os.Stat(destination); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("destination exists after byte-limit rejection: %v", statErr)
	}
}

func TestExtractZIPRejectsDuplicateCaseCollisionAndFileParent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fixtures []zipFixture
	}{
		{
			name: "duplicate",
			fixtures: []zipFixture{
				{name: "same", body: "one", mode: 0o600},
				{name: "same", body: "two", mode: 0o600},
			},
		},
		{
			name: "case collision",
			fixtures: []zipFixture{
				{name: "Readme", body: "one", mode: 0o600},
				{name: "README", body: "two", mode: 0o600},
			},
		},
		{
			name: "file parent",
			fixtures: []zipFixture{
				{name: "parent", body: "file", mode: 0o600},
				{name: "parent/child", body: "child", mode: 0o600},
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			archive := buildZIP(t, test.fixtures)
			_, err := ExtractZIP(
				bytes.NewReader(archive),
				int64(len(archive)),
				filepath.Join(t.TempDir(), "expanded"),
				ArchiveLimits{},
			)
			if !errors.Is(err, ErrUnsafeArchive) {
				t.Fatalf("ExtractZIP() error = %v, want ErrUnsafeArchive", err)
			}
		})
	}
}

func TestExtractZIPRequiresEmptyRealDestination(t *testing.T) {
	t.Parallel()

	archive := buildZIP(t, []zipFixture{{name: "safe", body: "data", mode: 0o600}})
	root := t.TempDir()
	destination := filepath.Join(root, "expanded")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "existing"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := ExtractZIP(bytes.NewReader(archive), int64(len(archive)), destination, ArchiveLimits{})
	if !errors.Is(err, ErrUnsafeArchive) {
		t.Fatalf("ExtractZIP() error = %v, want ErrUnsafeArchive", err)
	}
	assertFileContents(t, filepath.Join(destination, "existing"), "keep")
}

func TestExtractZIPFile(t *testing.T) {
	t.Parallel()

	archive := buildZIP(t, []zipFixture{{name: "safe.txt", body: "safe", mode: 0o644}})
	root := t.TempDir()
	archivePath := filepath.Join(root, "fixture.zip")
	if err := os.WriteFile(archivePath, archive, 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "expanded")

	stats, err := ExtractZIPFile(archivePath, destination, ArchiveLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Files != 1 || stats.ExpandedBytes != 4 {
		t.Fatalf("stats = %+v", stats)
	}
	assertFileContents(t, filepath.Join(destination, "safe.txt"), "safe")
}

func TestExtractZIPEmptyArchive(t *testing.T) {
	t.Parallel()

	archive := buildZIP(t, nil)
	destination := filepath.Join(t.TempDir(), "expanded")
	stats, err := ExtractZIP(bytes.NewReader(archive), int64(len(archive)), destination, ArchiveLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if stats != (ExtractStats{}) {
		t.Fatalf("stats = %+v", stats)
	}
	entries, err := os.ReadDir(destination)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("empty archive produced %d entries", len(entries))
	}
}

func buildZIP(t *testing.T, fixtures []zipFixture) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, fixture := range fixtures {
		header := &zip.FileHeader{
			Name:   fixture.name,
			Method: zip.Deflate,
		}
		header.SetMode(fixture.mode)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(entry, strings.NewReader(fixture.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func assertFileContents(t *testing.T, filePath, want string) {
	t.Helper()
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != want {
		t.Fatalf("%s = %q, want %q", filePath, content, want)
	}
}
