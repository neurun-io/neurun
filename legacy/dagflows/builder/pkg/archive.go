package pkg

import (
	"archive/zip"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/dagflows/builder/internal/domain"
)

func ZipDirectory(kind domain.ArtifactKind, name, srcDir, outDir string) (Artifact, error) {
	outPath := filepath.Join(outDir, name)
	file, err := os.Create(outPath)
	if err != nil {
		return Artifact{}, err
	}
	zipWriter := zip.NewWriter(file)

	walkErr := filepath.WalkDir(srcDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == srcDir {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		zipName := filepath.ToSlash(rel)
		if entry.IsDir() {
			_, err := zipWriter.Create(zipName + "/")
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = zipName
		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, source)
		closeErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	closeErr := zipWriter.Close()
	fileCloseErr := file.Close()
	if walkErr != nil {
		return Artifact{}, walkErr
	}
	if closeErr != nil {
		return Artifact{}, closeErr
	}
	if fileCloseErr != nil {
		return Artifact{}, fileCloseErr
	}

	return FileArtifact(kind, name, outPath, zipMediaType)
}
