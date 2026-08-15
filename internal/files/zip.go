package files

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	DefaultMaxArchiveEntries       = 1_000
	DefaultMaxArchiveExpandedBytes = 268_435_456
)

var (
	ErrUnsafeArchive       = errors.New("unsafe artifact archive")
	ErrTooManyArchiveFiles = errors.New("artifact archive has too many entries")
	ErrArchiveTooLarge     = errors.New("artifact archive exceeds expanded-byte limit")
)

// ArchiveLimits bounds both archive metadata and decompressed output. Zero
// values select conservative defaults.
type ArchiveLimits struct {
	MaxEntries       int
	MaxExpandedBytes int64
}

// ExtractStats describes the committed extraction.
type ExtractStats struct {
	Files         int
	Directories   int
	ExpandedBytes int64
}

type archiveEntry struct {
	file        *zip.File
	name        string
	isDirectory bool
	mode        fs.FileMode
}

// ExtractZIP validates the complete central directory before creating output,
// extracts into a same-directory staging area, and only then moves it into
// destination. Destination must not exist or must be an empty real directory.
func ExtractZIP(
	source io.ReaderAt,
	archiveSize int64,
	destination string,
	limits ArchiveLimits,
) (ExtractStats, error) {
	if source == nil {
		return ExtractStats{}, errors.New("artifact: ZIP source is nil")
	}
	if archiveSize < 0 {
		return ExtractStats{}, errors.New("artifact: ZIP size cannot be negative")
	}
	limits, err := normalizeArchiveLimits(limits)
	if err != nil {
		return ExtractStats{}, err
	}

	reader, err := zip.NewReader(source, archiveSize)
	if err != nil {
		return ExtractStats{}, err
	}
	entries, expectedBytes, stats, err := inspectArchive(reader.File, limits)
	if err != nil {
		return ExtractStats{}, err
	}

	absoluteDestination, err := validateDestination(destination)
	if err != nil {
		return ExtractStats{}, err
	}
	parent := filepath.Dir(absoluteDestination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return ExtractStats{}, err
	}
	if err := requireMissingOrEmptyDirectory(absoluteDestination); err != nil {
		return ExtractStats{}, err
	}

	staging, err := os.MkdirTemp(parent, "."+filepath.Base(absoluteDestination)+".extract-*")
	if err != nil {
		return ExtractStats{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.RemoveAll(staging)
		}
	}()

	// Directories first makes extraction independent of central-directory order.
	sort.SliceStable(entries, func(left, right int) bool {
		if entries[left].isDirectory != entries[right].isDirectory {
			return entries[left].isDirectory
		}
		return entries[left].name < entries[right].name
	})

	var expandedBytes int64
	for _, entry := range entries {
		target := filepath.Join(staging, filepath.FromSlash(entry.name))
		if !pathWithin(staging, target) {
			return ExtractStats{}, fmt.Errorf("%w: entry escapes staging directory", ErrUnsafeArchive)
		}
		if entry.isDirectory {
			if err := os.MkdirAll(target, directoryMode(entry.mode)); err != nil {
				return ExtractStats{}, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return ExtractStats{}, err
		}

		sourceFile, err := entry.file.Open()
		if err != nil {
			return ExtractStats{}, err
		}
		targetFile, err := os.OpenFile(
			target,
			os.O_CREATE|os.O_EXCL|os.O_WRONLY,
			regularFileMode(entry.mode),
		)
		if err != nil {
			_ = sourceFile.Close()
			return ExtractStats{}, err
		}

		remaining := limits.MaxExpandedBytes - expandedBytes
		result, copyErr := CopyAndHash(targetFile, sourceFile, remaining)
		targetCloseErr := targetFile.Close()
		sourceCloseErr := sourceFile.Close()
		if copyErr != nil {
			if errors.Is(copyErr, ErrByteLimitExceeded) {
				return ExtractStats{}, fmt.Errorf("%w: limit %d",
					ErrArchiveTooLarge, limits.MaxExpandedBytes)
			}
			return ExtractStats{}, copyErr
		}
		if targetCloseErr != nil {
			return ExtractStats{}, targetCloseErr
		}
		if sourceCloseErr != nil {
			return ExtractStats{}, sourceCloseErr
		}
		if uint64(result.SizeBytes) != entry.file.UncompressedSize64 {
			return ExtractStats{}, fmt.Errorf("%w: entry size differs from ZIP metadata", ErrUnsafeArchive)
		}
		expandedBytes += result.SizeBytes
	}

	if expandedBytes != expectedBytes {
		return ExtractStats{}, fmt.Errorf("%w: expanded size differs from ZIP metadata", ErrUnsafeArchive)
	}
	if err := commitStaging(staging, absoluteDestination); err != nil {
		return ExtractStats{}, err
	}
	committed = true
	stats.ExpandedBytes = expandedBytes
	return stats, nil
}

// ExtractZIPFile opens a regular archive and delegates to ExtractZIP.
func ExtractZIPFile(
	archivePath string,
	destination string,
	limits ArchiveLimits,
) (ExtractStats, error) {
	file, err := os.Open(archivePath)
	if err != nil {
		return ExtractStats{}, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return ExtractStats{}, err
	}
	if !info.Mode().IsRegular() {
		return ExtractStats{}, fmt.Errorf("artifact: ZIP source is not a regular file")
	}
	return ExtractZIP(file, info.Size(), destination, limits)
}

func normalizeArchiveLimits(limits ArchiveLimits) (ArchiveLimits, error) {
	if limits.MaxEntries < 0 || limits.MaxExpandedBytes < 0 {
		return ArchiveLimits{}, errors.New("artifact: archive limits cannot be negative")
	}
	if limits.MaxEntries == 0 {
		limits.MaxEntries = DefaultMaxArchiveEntries
	}
	if limits.MaxExpandedBytes == 0 {
		limits.MaxExpandedBytes = DefaultMaxArchiveExpandedBytes
	}
	return limits, nil
}

func inspectArchive(
	files []*zip.File,
	limits ArchiveLimits,
) ([]archiveEntry, int64, ExtractStats, error) {
	if len(files) > limits.MaxEntries {
		return nil, 0, ExtractStats{}, fmt.Errorf("%w: entries %d, limit %d",
			ErrTooManyArchiveFiles, len(files), limits.MaxEntries)
	}

	entries := make([]archiveEntry, 0, len(files))
	types := make(map[string]bool, len(files))
	caseFolded := make(map[string]string, len(files))
	var expectedBytes uint64
	var stats ExtractStats

	for _, file := range files {
		name, err := safeArchiveName(file.Name)
		if err != nil {
			return nil, 0, ExtractStats{}, err
		}
		mode := file.Mode()
		isDirectory := file.FileInfo().IsDir() || strings.HasSuffix(file.Name, "/")
		if mode&os.ModeSymlink != 0 {
			return nil, 0, ExtractStats{}, fmt.Errorf("%w: symlink entry", ErrUnsafeArchive)
		}
		if isDirectory {
			if file.UncompressedSize64 != 0 {
				return nil, 0, ExtractStats{}, fmt.Errorf("%w: directory entry contains data", ErrUnsafeArchive)
			}
		} else if mode.Type() != 0 {
			return nil, 0, ExtractStats{}, fmt.Errorf("%w: non-regular entry", ErrUnsafeArchive)
		}

		folded := strings.ToLower(name)
		if previous, exists := caseFolded[folded]; exists {
			return nil, 0, ExtractStats{}, fmt.Errorf(
				"%w: duplicate or case-colliding entries %q and %q",
				ErrUnsafeArchive, previous, name,
			)
		}
		caseFolded[folded] = name
		types[name] = isDirectory

		if isDirectory {
			stats.Directories++
		} else {
			stats.Files++
			if file.UncompressedSize64 > uint64(limits.MaxExpandedBytes) ||
				expectedBytes > uint64(limits.MaxExpandedBytes)-file.UncompressedSize64 {
				return nil, 0, ExtractStats{}, fmt.Errorf("%w: limit %d",
					ErrArchiveTooLarge, limits.MaxExpandedBytes)
			}
			expectedBytes += file.UncompressedSize64
		}
		entries = append(entries, archiveEntry{
			file:        file,
			name:        name,
			isDirectory: isDirectory,
			mode:        mode,
		})
	}

	// Reject "file" plus "file/child" conflicts before writing anything.
	for name := range types {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if isDirectory, exists := types[parent]; exists && !isDirectory {
				return nil, 0, ExtractStats{}, fmt.Errorf(
					"%w: regular file is parent of another entry",
					ErrUnsafeArchive,
				)
			}
		}
	}
	return entries, int64(expectedBytes), stats, nil
}

func safeArchiveName(rawName string) (string, error) {
	if rawName == "" || !utf8.ValidString(rawName) {
		return "", fmt.Errorf("%w: empty or invalid UTF-8 entry name", ErrUnsafeArchive)
	}
	if strings.ContainsAny(rawName, "\\\x00:") ||
		strings.HasPrefix(rawName, "/") {
		return "", fmt.Errorf("%w: absolute or non-portable entry name", ErrUnsafeArchive)
	}
	for _, character := range rawName {
		if unicode.IsControl(character) {
			return "", fmt.Errorf("%w: entry name contains a control character", ErrUnsafeArchive)
		}
	}

	name := strings.TrimSuffix(rawName, "/")
	if name == "" {
		return "", fmt.Errorf("%w: root directory entry", ErrUnsafeArchive)
	}
	components := strings.Split(name, "/")
	for _, component := range components {
		if component == "" ||
			component == "." ||
			component == ".." ||
			component != strings.TrimRight(component, ". ") ||
			isWindowsDeviceName(component) {
			return "", fmt.Errorf("%w: traversal or ambiguous entry name", ErrUnsafeArchive)
		}
	}
	cleaned := path.Clean(name)
	if cleaned != name || path.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("%w: traversal entry name", ErrUnsafeArchive)
	}
	return cleaned, nil
}

func validateDestination(destination string) (string, error) {
	if strings.TrimSpace(destination) == "" {
		return "", errors.New("artifact: extraction destination is required")
	}
	absolute, err := filepath.Abs(destination)
	if err != nil {
		return "", err
	}
	absolute = filepath.Clean(absolute)
	volumeRoot := filepath.Clean(filepath.VolumeName(absolute) + string(filepath.Separator))
	if absolute == volumeRoot {
		return "", fmt.Errorf("%w: filesystem root cannot be an extraction destination", ErrUnsafeArchive)
	}
	return absolute, nil
}

func requireMissingOrEmptyDirectory(destination string) error {
	info, err := os.Lstat(destination)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("%w: destination must be a real directory", ErrUnsafeArchive)
	}
	entries, err := os.ReadDir(destination)
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("%w: destination directory must be empty", ErrUnsafeArchive)
	}
	return nil
}

func commitStaging(staging, destination string) error {
	info, err := os.Lstat(destination)
	switch {
	case errors.Is(err, os.ErrNotExist):
		// Nothing to remove.
	case err != nil:
		return err
	case info.Mode()&os.ModeSymlink != 0 || !info.IsDir():
		return fmt.Errorf("%w: destination changed during extraction", ErrUnsafeArchive)
	default:
		entries, readErr := os.ReadDir(destination)
		if readErr != nil {
			return readErr
		}
		if len(entries) != 0 {
			return fmt.Errorf("%w: destination changed during extraction", ErrUnsafeArchive)
		}
		if err := os.Remove(destination); err != nil {
			return err
		}
	}

	if err := os.Rename(staging, destination); err != nil {
		// Preserve the caller's expectation that a pre-created destination
		// remains available if the final rename fails.
		_ = os.Mkdir(destination, 0o755)
		return err
	}
	return nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relative)
}

func regularFileMode(mode fs.FileMode) fs.FileMode {
	permissions := mode.Perm() & 0o755
	return permissions | 0o600
}

func directoryMode(mode fs.FileMode) fs.FileMode {
	permissions := mode.Perm() & 0o755
	return permissions | 0o700
}

func isWindowsDeviceName(component string) bool {
	base := component
	if index := strings.IndexByte(base, '.'); index >= 0 {
		base = base[:index]
	}
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return true
	}
	return false
}
