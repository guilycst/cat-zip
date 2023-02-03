package main

import (
	"archive/zip"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var unzipedFiles map[string]uint = make(map[string]uint)
var catFile *os.File

func main() {
	var dir = flag.String("dir", ".", "Directory where the input zip files are placed")
	var outdir = flag.String("outdir", ".", "Directory where the output unziped files will be placed")
	var ext = flag.String("ext", ".gz", "Filter input files by extension: .zip and .gz")
	var outdirCatFileName = flag.String("outfile", "unknown_blob", "Concatenated file containing all of the unziped files content")
	var help = flag.Bool("help", false, "Show help")
	flag.Parse()

	if *help {
		flag.Usage()
		os.Exit(0)
	}

	filesInDir := []string{}
	filepath.WalkDir(*dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		if filepath.Ext(d.Name()) == *ext {
			filesInDir = append(filesInDir, path)
		}

		return nil
	})

	catFilePath := filepath.Join(*outdir, *outdirCatFileName)
	catFile, _ = os.OpenFile(catFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	defer catFile.Close()

	switch filepath.Ext(*ext) {
	case ".gz":
		handleGz(filesInDir, ext, outdir)
		break
	default:
		handleZip(filesInDir, ext, outdir)
		break
	}

}

func handleGz(filesInDir []string, ext *string, outdir *string) {
	for _, gzFilename := range filesInDir {

		newFilename := strings.TrimSuffix(gzFilename, ".gz")
		newFilename = autoRenameRepeatedFiles(newFilename)

		writer, err := os.Create(newFilename)
		if err != nil {
			log.Fatal(err)
		}
		defer writer.Close()

		err = copyFileGz(gzFilename, newFilename, writer)
		if err != nil {
			log.Fatal(err)
		}

		err = copyFileGz(gzFilename, newFilename, catFile)
		if err != nil {
			log.Fatal(err)
		}
		catFile.WriteString("\n")
	}
}

func copyFileGz(gzFilename string, newFilename string, writer io.WriteCloser) error {

	gzFile, err := os.Open(gzFilename)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer gzFile.Close()

	reader, err := gzip.NewReader(gzFile)
	if err != nil {
		return err
	}
	defer reader.Close()

	if err = ioCopy(newFilename, writer, reader); err != nil {
		return err
	}

	return nil
}

func handleZip(filesInDir []string, ext *string, outdir *string) {
	for _, f := range filesInDir {
		reader, err := zip.OpenReader(f)
		if err != nil {
			log.Fatalf("Unable to read %s file ", *ext)
		}
		defer reader.Close()

		destination, err := filepath.Abs(*outdir)
		if err != nil {
			log.Fatalf("Unable to find absolute path for dir %s ", *outdir)
		}

		for _, f := range reader.File {
			err := unzipFile(f, destination)
			if err != nil {
				log.Fatal("Unable to to unzip file inside archive: ", err)
			}
		}
	}
}

func autoRenameRepeatedFiles(filePath string) string {
	counter, repeated := unzipedFiles[filePath]
	if repeated {
		dir := filepath.Dir(filePath)
		ext := filepath.Ext(filePath)
		fileName := filepath.Base(filePath)
		fileName = fileName[:len(fileName)-len(ext)]
		fileName = fmt.Sprintf("%s(%d)%s", fileName, counter, ext)
		filePath = filepath.Join(dir, fileName)
	}
	return filePath
}

func unzipFile(f *zip.File, destination string) error {
	//Check if file paths are not vulnerable to Zip Slip
	filePath := filepath.Join(destination, f.Name)
	if !strings.HasPrefix(filePath, filepath.Clean(destination)+string(os.PathSeparator)) {
		return fmt.Errorf("invalid file path: %s", filePath)
	}

	// Not needed but will create directory tree
	if f.FileInfo().IsDir() {
		if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
			return err
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
		return err
	}

	// The ziped files migh have files with the same name, solving that
	counter, repeated := unzipedFiles[filePath]
	if repeated {
		dir := filepath.Dir(filePath)
		ext := filepath.Ext(filePath)
		fileName := filepath.Base(filePath)
		fileName = fileName[:len(fileName)-len(ext)]
		fileName = fmt.Sprintf("%s(%d)%s", fileName, counter, ext)
		filePath = filepath.Join(dir, fileName)
	}

	// 6. Create a destination file for unzipped content
	destinationFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}

	defer destinationFile.Close()

	if err = copyToFile(f, destinationFile); err != nil {
		return err
	}

	//Apend to cat
	if err = copyToFile(f, catFile); err != nil {
		return err
	}
	catFile.WriteString("\n")

	unzipedFiles[filePath] += 1
	return nil
}

func copyToFile(f *zip.File, destinationFile *os.File) error {
	zippedFile, err := f.Open()
	if err != nil {
		return err
	}
	defer zippedFile.Close()

	if err = ioCopy(destinationFile.Name(), destinationFile, zippedFile); err != nil {
		return err
	}

	log.Printf("output file at %v", destinationFile.Name())
	return nil
}

func ioCopy(filename string, writer io.Writer, reader io.ReadCloser) error {
	if _, err := io.Copy(writer, reader); err != nil {
		return err
	}
	log.Printf("output file at %v", filename)
	return nil
}
