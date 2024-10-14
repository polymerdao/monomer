package monogen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cometos "github.com/cometbft/cometbft/libs/os"
	"github.com/gobuffalo/genny/v2"
	"github.com/ignite/cli/v28/ignite/config"
	"github.com/ignite/cli/v28/ignite/pkg/cache"
	"github.com/ignite/cli/v28/ignite/pkg/gocmd"
	"github.com/ignite/cli/v28/ignite/pkg/placeholder"
	"github.com/ignite/cli/v28/ignite/pkg/xast"
	"github.com/ignite/cli/v28/ignite/services/scaffolder"
	"github.com/ignite/cli/v28/ignite/templates/module"
	"github.com/ignite/cli/v28/ignite/version"
	"golang.org/x/mod/modfile"
)

func Generate(ctx context.Context, appDirPath, goModulePath, addressPrefix string, skipGit, isTest bool) error {
	if cometos.FileExists(appDirPath) {
		return fmt.Errorf("refusing to overwrite directory: %s", appDirPath)
	}

	igniteRootDir, err := config.DirPath()
	if err != nil {
		return fmt.Errorf("get ignite config directory: %v", err)
	}
	// https://github.com/ignite/cli/blob/2a968e8684cae0a1d79ccb11f0db067a4605173e/ignite/cmd/cmd.go#L33-L34
	cacheDBPath := filepath.Join(igniteRootDir, "ignite_cache.db")
	cacheStorage, err := cache.NewStorage(cacheDBPath, cache.WithVersion(version.Version))
	if err != nil {
		return fmt.Errorf("new ignite cache storage: %v", err)
	}

	// Initial ignite scaffolding.
	appDir, err := scaffolder.Init(
		ctx,
		cacheStorage,
		placeholder.New(),
		appDirPath,
		goModulePath,
		addressPrefix,
		true, // no default module
		skipGit,
		false, // skip proto
		true,  // minimal
		false, // consumer chain (ics)
		nil,   // params
	)
	if err != nil {
		return fmt.Errorf("use ignite to scaffold initial cosmos sdk project: %v", err)
	}

	// monogen modifications.
	g := genny.New()
	g.RunFn(func(r *genny.Runner) error {
		appGoPath := filepath.Join(appDir, "app", "app.go")
		appConfigGoPath := filepath.Join(appDir, "app", "app_config.go")
		if err := addRollupModule(r, appGoPath, appConfigGoPath); err != nil {
			return fmt.Errorf("add rollup module: %v", err)
		}
		if err := removeConsensusModule(r, appGoPath, appConfigGoPath); err != nil {
			return fmt.Errorf("remove consensus module: %v", err)
		}
		appName := filepath.Base(appDir)
		if err := addMonomerCommand(r, filepath.Join(appDir, "cmd", appName+"d", "cmd", "commands.go")); err != nil {
			return fmt.Errorf("add monomer command: %v", err)
		}
		if err := addReplaceDirectives(r, filepath.Join(appDir, "go.mod"), isTest); err != nil {
			return fmt.Errorf("add replace directives: %v", err)
		}
		return nil
	})
	r := genny.WetRunner(ctx)
	if err := r.With(g); err != nil {
		return fmt.Errorf("attach genny generator to wet runner: %v", err)
	}
	if err := r.Run(); err != nil {
		return fmt.Errorf("apply monogen modifications: %v", err)
	}

	if err := gocmd.Fmt(ctx, appDir); err != nil {
		return fmt.Errorf("go fmt: %v", err)
	}
	if err := gocmd.GoImports(ctx, appDir); err != nil {
		return fmt.Errorf("run goimports: %v", err)
	}
	if err := gocmd.ModTidy(ctx, appDir); err != nil {
		return fmt.Errorf("go mod tidy: %v", err)
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
	goModBytes, err := os.ReadFile(goModPath)
	if err != nil {
		return fmt.Errorf("read go.mod: %v", err)
	}

	mf, err := modfile.Parse(goModPath, goModBytes, nil)
	if err != nil {
		return fmt.Errorf("parse go.mod: %v", err)
	}

	dependencies := []string{
		"github.com/crate-crypto/go-kzg-4844@v0.7.0",
		"github.com/quic-go/quic-go@v0.39.3",
		"github.com/quic-go/webtransport-go@v0.6.0",
	}

	for _, dep := range dependencies {
		modulePath, version, _ := strings.Cut(dep, "@")
		if err := mf.AddRequire(modulePath, version); err != nil {
			return fmt.Errorf("add require %s: %v", dep, err)
		}
	}

	if isTest {
		currentDir, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get current working directory: %v", err)
		}
		currentDirAbs, err := filepath.Abs(currentDir)
		if err != nil {
			return fmt.Errorf("get absolute path: %v", err)
		}
		mf.AddReplace("github.com/polymerdao/monomer", "", filepath.Dir(currentDirAbs), "")
	}

	newGoModBytes, err := mf.Format()
	if err != nil {
		return fmt.Errorf("format go.mod: %v", err)
	}

	if err := r.File(genny.NewFileS(goModPath, string(newGoModBytes))); err != nil {
		return fmt.Errorf("write %s: %v", goModPath, err)
	}

	return nil
}
