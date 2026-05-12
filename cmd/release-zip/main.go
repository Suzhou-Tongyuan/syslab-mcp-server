package main

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 3 {
		fatalf("usage: %s <zip-path> <source-path[:archive-name]>...\n", filepath.Base(os.Args[0]))
	}

	zipPath := os.Args[1]
	if err := os.MkdirAll(filepath.Dir(zipPath), 0o755); err != nil {
		fatalf("create zip directory: %v\n", err)
	}

	outFile, err := os.Create(zipPath)
	if err != nil {
		fatalf("create zip file: %v\n", err)
	}
	defer outFile.Close()

	writer := zip.NewWriter(outFile)
	defer writer.Close()

	for _, spec := range os.Args[2:] {
		sourcePath, archiveName := parseSpec(spec)
		if err := addFile(writer, sourcePath, archiveName); err != nil {
			fatalf("add %s: %v\n", sourcePath, err)
		}
	}

	if err := writer.Close(); err != nil {
		fatalf("close zip writer: %v\n", err)
	}
	if err := outFile.Close(); err != nil {
		fatalf("close zip file: %v\n", err)
	}

	fmt.Printf("Created %s\n", zipPath)
}

func parseSpec(spec string) (string, string) {
	sourcePath, archiveName, found := strings.Cut(spec, ":")
	if !found || archiveName == "" {
		return spec, filepath.Base(spec)
	}
	return sourcePath, archiveName
}

func addFile(writer *zip.Writer, sourcePath string, archiveName string) error {
	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("directories are not supported")
	}

	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	header.Name = filepath.ToSlash(archiveName)
	header.Method = zip.Deflate

	fileWriter, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}

	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	_, err = io.Copy(fileWriter, sourceFile)
	return err
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}
