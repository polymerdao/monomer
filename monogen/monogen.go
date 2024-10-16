package monogen

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/polymerdao/monomer/utils"
	"golang.org/x/mod/modfile"
)

//go:embed testapp.zip
var appZip []byte

func Generate(ctx context.Context, appDirPath, goModulePath, addressPrefix, monomerPath string) (err error) {
	if _, err := os.Stat(appDirPath); err == nil {
		return fmt.Errorf("refusing to overwrite: %s", appDirPath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat: %v", err)
	}

	appName := filepath.Base(appDirPath)
	appRoot := filepath.Dir(appDirPath)

	// Unzip.
	zipReader, err := zip.NewReader(bytes.NewReader(appZip), int64(len(appZip)))
	if err != nil {
		return fmt.Errorf("create zip reader: %v", err)
	}
	for _, file := range zipReader.File {
		filePath := filepath.Join(appRoot, strings.ReplaceAll(file.Name, "testapp", appName))
		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(filePath, file.Mode()); err != nil {
				return fmt.Errorf("create directory: %v", err)
			}
			continue
		}
		if err := writeFile(file, filePath); err != nil {
			return err
		}
	}

	if err := filepath.Walk(appDirPath, func(path string, info fs.FileInfo, err error) error {
		return replaceModulePath(path, goModulePath, appName, info, err)
	}); err != nil {
		return fmt.Errorf("replace module path everywhere: %v", err)
	}

	appGoPath := filepath.Join(appDirPath, "app", "app.go")
	appGoBytes, err := os.ReadFile(appGoPath)
	if err != nil {
		return fmt.Errorf("read file: %v", err)
	}
	newAppGoString := strings.Replace(
		string(appGoBytes),
		`AccountAddressPrefix = "cosmos"`,
		fmt.Sprintf(`AccountAddressPrefix = %q`, addressPrefix),
		1,
	)
	if err := os.WriteFile(appGoPath, []byte(newAppGoString), 0); err != nil {
		return fmt.Errorf("write file: %v", err)
	}

	if monomerPath == "" {
		return nil
	}

	// Add replace to go.mod.
	goModPath := filepath.Join(appDirPath, "go.mod")
	goModBytes, err := os.ReadFile(goModPath)
	if err != nil {
		return fmt.Errorf("read file: %v", err)
	}
	goMod, err := modfile.Parse(goModPath, goModBytes, nil)
	if err != nil {
		return fmt.Errorf("parse: %v", err)
	}
	monomerPathAbs, err := filepath.Abs(monomerPath)
	if err != nil {
		return fmt.Errorf("absolute monomer path: %v", err)
	}
	if err := goMod.AddReplace("github.com/polymerdao/monomer", "", monomerPathAbs, ""); err != nil {
		return fmt.Errorf("add replace: %v", err)
	}
	goModBytes, err = goMod.Format()
	if err != nil {
		return fmt.Errorf("format: %v", err)
	}
	if err := os.WriteFile(goModPath, goModBytes, 0); err != nil {
		return fmt.Errorf("write file: %v", err)
	}

	return nil
}

func replaceModulePath(path, modulePath, appName string, fileInfo fs.FileInfo, err error) error {
	if err != nil {
		return err
	}
	if fileInfo.IsDir() {
		return nil
	}
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read file: %v", err)
	}
	newString := strings.ReplaceAll(string(fileBytes), "github.com/polymerdao/monomer/monogen/testapp", modulePath)
	newString = strings.ReplaceAll(newString, "testapp", appName)
	if err := os.WriteFile(path, []byte(newString), fileInfo.Mode()); err != nil {
		return fmt.Errorf("write file: %v", err)
	}
	return nil
}

func writeFile(file *zip.File, outPath string) (err error) {
	outFile, err := os.OpenFile(outPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, file.Mode())
	if err != nil {
		return fmt.Errorf("open file: %v", err)
	}
	defer func() {
		err = utils.WrapCloseErr(err, outFile)
	}()

	rc, err := file.Open()
	if err != nil {
		return fmt.Errorf("open file in zip: %v", err)
	}
	defer func() {
		err = utils.WrapCloseErr(err, rc)
	}()
	if _, err = io.Copy(outFile, rc); err != nil { //nolint:gosec // G110: decompression bomb. Not a concern since we know the payload.
		return fmt.Errorf("copy file content: %v", err)
	}
	return nil
}
