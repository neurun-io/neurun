package builder

import (
	"archive/zip"
	"io"
	"os"
	"time"
)

// zipFileAs packages one file under a chosen name, executable.
//
// Compiled runtimes ship a binary rather than a tree, and the mode has to
// survive the round trip: extraction masks the stored permissions down to 0755,
// so a file written 0644 comes back unrunnable.
func zipFileAs(sourcePath, name, target string) error {
	input, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer input.Close()

	output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(output)
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(0o755)
	header.Modified = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	writer, err := archive.CreateHeader(header)
	if err == nil {
		_, err = io.Copy(writer, input)
	}
	if err != nil {
		archive.Close()
		output.Close()
		return err
	}
	if err := archive.Close(); err != nil {
		output.Close()
		return err
	}
	return output.Close()
}
