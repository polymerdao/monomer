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

	"golang.org/x/mod/modfile"

	"github.com/gobuffalo/genny/v2"
	"github.com/ignite/cli/v28/ignite/pkg/placeholder"
	"github.com/ignite/cli/v28/ignite/pkg/xast"
	"github.com/ignite/cli/v28/ignite/templates/module"
	"github.com/polymerdao/monomer/utils"
)

//go:embed testapp.zip
var appZip []byte

func Generate(ctx context.Context, appDirPath, goModulePath, addressPrefix string, monomerPath string) (err error) {
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
		filePath := filepath.Join(appRoot, strings.Replace(file.Name, "testapp", appName, -1))
		// TODO vuln thing?
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
		return replaceModulePath(path, goModulePath, info, err)
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
		`	AccountAddressPrefix = "cosmos"
	Name                 = "testapp"`, // TODODODODOD replace all occrences of testapp -- something with tests as well
		fmt.Sprintf(`	AccountAddressPrefix = "%s"
	Name                 = "%s"`, addressPrefix, appName), 1)
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
	if err := goMod.AddReplace(goModulePath, "", monomerPath, ""); err != nil {
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

func replaceModulePath(path, modulePath string, fileInfo fs.FileInfo, err error) error {
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
	newString := strings.Replace(string(fileBytes), "github.com/polymerdao/monomer/monogen/testapp", modulePath, -1)
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
	if _, err = io.Copy(outFile, rc); err != nil {
		return fmt.Errorf("copy file content: %v", err)
	}
	return nil
}

func addRollupModule(r *genny.Runner, appGoPath, appConfigGoPath string) error {
	replacer := placeholder.New()

	// Modify the app config, usually at app/app_config.go.
	appConfigGo, err := r.Disk.Find(appConfigGoPath)
	if err != nil {
		return fmt.Errorf("find: %v", err)
	}

	// 1. Import the required rollup module packages.
	content, err := xast.AppendImports(
		appConfigGo.String(),
		xast.WithLastNamedImport("rolluptypes", "github.com/polymerdao/monomer/x/rollup/types"),
		xast.WithLastNamedImport("rollupmodulev1", "github.com/polymerdao/monomer/gen/rollup/module/v1"),
	)
	if err != nil {
		return fmt.Errorf("append rollup module imports to %s: %v", appConfigGoPath, err)
	}

	// 2. Modify rollup module account permissions.
	content = replacer.Replace(
		content,
		module.PlaceholderSgAppMaccPerms,
		fmt.Sprintf(`{Account: rolluptypes.ModuleName, Permissions: []string{authtypes.Minter, authtypes.Burner}},
		%s`, module.PlaceholderSgAppMaccPerms),
	)

	// 3. Add rollup module to app config.
	content = replacer.Replace(content, module.PlaceholderSgAppModuleConfig, fmt.Sprintf(`{
				Name:   rolluptypes.ModuleName,
				Config: appconfig.WrapAny(&rollupmodulev1.Module{}),
			},
			%s`, module.PlaceholderSgAppModuleConfig))

	if err := r.File(genny.NewFileS(appConfigGoPath, content)); err != nil {
		return fmt.Errorf("write %s: %v", appConfigGoPath, err)
	}

	// Modify the main application file, usually at app/app.go.
	appGo, err := r.Disk.Find(appGoPath)
	if err != nil {
		return fmt.Errorf("find: %v", err)
	}

	// 1. Import the required rollup module packages.
	content, err = xast.AppendImports(
		appGo.String(),
		xast.WithLastNamedImport("rollupkeeper", "github.com/polymerdao/monomer/x/rollup/keeper"),
		xast.WithLastNamedImport("_", "github.com/polymerdao/monomer/x/rollup"),
	)
	if err != nil {
		return fmt.Errorf("append rollup module imports to %s: %v", appGoPath, err)
	}

	// 2. Add rollup module keeper declaration.
	keeperDeclaration := fmt.Sprintf(`RollupKeeper *rollupkeeper.Keeper
	%s`, module.PlaceholderSgAppKeeperDeclaration)
	content = replacer.Replace(content, module.PlaceholderSgAppKeeperDeclaration, keeperDeclaration)

	// 3. Add rollup module keeper definition.
	keeperDefinition := fmt.Sprintf(`&app.RollupKeeper,
		%s`, module.PlaceholderSgAppKeeperDefinition)
	content = replacer.Replace(content, module.PlaceholderSgAppKeeperDefinition, keeperDefinition)

	if err := r.File(genny.NewFileS(appGoPath, content)); err != nil {
		return fmt.Errorf("write %s: %v", appGoPath, err)
	}

	return nil
}

func removeConsensusModule(r *genny.Runner, appGoPath, appConfigGoPath string) error {
	replacer := placeholder.New()

	// Modify the app config, usually at app/app_config.go.
	appConfigGo, err := r.Disk.Find(appConfigGoPath)
	if err != nil {
		return fmt.Errorf("find: %v", err)
	}

	// 1. Remove import.
	content := replacer.Replace(appConfigGo.String(), `consensusmodulev1 "cosmossdk.io/api/cosmos/consensus/module/v1"
	`, "")
	content = replacer.Replace(content, `consensustypes "github.com/cosmos/cosmos-sdk/x/consensus/types"
	`, "")

	// 2. Remove module configuration.
	content = replacer.Replace(content, `
			{
				Name:   consensustypes.ModuleName,
				Config: appconfig.WrapAny(&consensusmodulev1.Module{}),
			},`, "")

	if err := r.File(genny.NewFileS(appConfigGoPath, content)); err != nil {
		return fmt.Errorf("write %s: %v", appConfigGoPath, err)
	}

	// Modify the app file, usually at app/app.go.
	appGo, err := r.Disk.Find(appGoPath)
	if err != nil {
		return fmt.Errorf("find: %v", err)
	}

	// 1. Remove import.
	content = replacer.Replace(appGo.String(), `_ "github.com/cosmos/cosmos-sdk/x/consensus" // import for side-effects
	`, "")
	content = replacer.Replace(content, `consensuskeeper "github.com/cosmos/cosmos-sdk/x/consensus/keeper"
	`, "")

	// 2. Remove keeper declaration.
	content = replacer.Replace(content, `ConsensusParamsKeeper consensuskeeper.Keeper
`, "")

	// 3. Remove keeper definition.
	content = replacer.Replace(content, `&app.ConsensusParamsKeeper,
		`, "")

	if err := r.File(genny.NewFileS(appGoPath, content)); err != nil {
		return fmt.Errorf("write %s: %v", appGoPath, err)
	}

	return nil
}

func addMonomerCommand(r *genny.Runner, commandsGoPath string) error {
	replacer := placeholder.New()

	commandsGo, err := r.Disk.Find(commandsGoPath)
	if err != nil {
		return fmt.Errorf("find: %v", err)
	}

	content, err := xast.AppendImports(commandsGo.String(), xast.WithLastImport("github.com/polymerdao/monomer/integrations"))
	if err != nil {
		return fmt.Errorf("append import: %v", err)
	}

	content = replacer.Replace(content, `
	server.AddCommands(rootCmd, app.DefaultNodeHome, newApp, appExport, addModuleInitFlags)`, `
	integrations.AddMonomerCommand(rootCmd, newApp, app.DefaultNodeHome)`)

	if err := r.File(genny.NewFileS(commandsGoPath, content)); err != nil {
		return fmt.Errorf("write %s: %v", commandsGoPath, err)
	}

	return nil
}

func addReplaceDirectives(r *genny.Runner, goModPath string, isTest bool) error {
	replacer := placeholder.New()

	goMod, err := r.Disk.Find(goModPath)
	if err != nil {
		return fmt.Errorf("find: %v", err)
	}
	var monomerReplace string
	if isTest {
		currentDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get current working directory: %v", err)
		}
		currentDirAbs, err := filepath.Abs(currentDir)
		if err != nil {
			return fmt.Errorf("get absolute path: %v", err)
		}
		monomerReplace = fmt.Sprintf("github.com/polymerdao/monomer => %s", filepath.Dir(currentDirAbs))
	}
	content := replacer.Replace(goMod.String(), "replace (", fmt.Sprintf(`replace (
	cosmossdk.io/core => cosmossdk.io/core v0.11.1
	github.com/btcsuite/btcd/btcec/v2 v2.3.4 => github.com/btcsuite/btcd/btcec/v2 v2.3.2
	github.com/crate-crypto/go-ipa => github.com/crate-crypto/go-ipa v0.0.0-20231205143816-408dbffb2041
	github.com/crate-crypto/go-kzg-4844 v1.0.0 => github.com/crate-crypto/go-kzg-4844 v0.7.0
	github.com/ethereum/go-ethereum => github.com/joshklop/op-geth v0.0.0-20240515205036-e3b990384a74
	github.com/libp2p/go-libp2p => github.com/joshklop/go-libp2p v0.0.0-20241004015633-cfc9936c6811
	github.com/quic-go/quic-go => github.com/quic-go/quic-go v0.39.3
	github.com/quic-go/webtransport-go => github.com/quic-go/webtransport-go v0.6.0
	%s
	`, monomerReplace))
	if err := r.File(genny.NewFileS(goModPath, content)); err != nil {
		return fmt.Errorf("write %s: %v", goModPath, err)
	}
	return nil
}
