package monogen

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/gobuffalo/genny/v2"
	"github.com/ignite/cli/v28/ignite/config"
	"github.com/ignite/cli/v28/ignite/pkg/cache"
	"github.com/ignite/cli/v28/ignite/pkg/placeholder"
	"github.com/ignite/cli/v28/ignite/pkg/xast"
	"github.com/ignite/cli/v28/ignite/services/scaffolder"
	"github.com/ignite/cli/v28/ignite/templates/module"
)

func Generate(ctx context.Context, goModulePath, addressPrefix string, skipGit bool) error {
	igniteRootDir, err := config.DirPath()
	if err != nil {
		return fmt.Errorf("get ignite config directory: %v", err)
	}
	// https://github.com/ignite/cli/blob/2a968e8684cae0a1d79ccb11f0db067a4605173e/ignite/cmd/cmd.go#L33-L34
	cacheDBPath := filepath.Join(igniteRootDir, "ignite_cache.db")
	// Note that we need to change this version every time we bump ignite's version in Monomer's go.mod.
	cacheStorage, err := cache.NewStorage(cacheDBPath, cache.WithVersion("v28.5.1"))
	if err != nil {
		return fmt.Errorf("new ignite cache storage: %v", err)
	}

	// Initial ignite scaffolding.
	appDir, err := scaffolder.Init(
		ctx,
		cacheStorage,
		placeholder.New(),
		"", // app directory path (empty means ignite will use the goModulePath)
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
		if err := addRollupModule(r, filepath.Join(appDir, "app", "app.go"), filepath.Join(appDir, "app", "app_config.go")); err != nil {
			return fmt.Errorf("add rollup module: %v", err)
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

	// go mod tidy
	// go fmt

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
		`{Account: rolluptypes.ModuleName, Permissions: []string{authtypes.Minter, authtypes.Burner}},`,
	)

	// 3. Add rollup module to app config.
	content = replacer.Replace(content, module.PlaceholderSgAppModuleConfig, `{
		Name:   rolluptypes.ModuleName,
		Config: appconfig.WrapAny(rollupmodulev1.Module{}),
	}`)

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
		xast.WithLastNamedImport("_", "github.com/polymerdao/monomer/x/rollup // import for side-effects"),
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

// remove consensus module
//   - app.go
//   - app_config.go

// add monomer command
//   - cmd/{name}d/commands.go

// replace directives
//   - go.mod

// to test, we run Generate and then run all of the unit tests + run monomer start + hit a comet endpoint
