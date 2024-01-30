package server

import (
	"net/http"
	"os"
	"runtime/pprof"
)

func StartCpuProfiler(config *Config) (func(), error) {
	empty := func() {}
	if config.CpuProfile == "" {
		return empty, nil
	}

	f, err := os.Create(config.CpuProfile)
	if err != nil {
		return empty, err
	}

	config.Logger.Info("starting CPU profiler", "profile", config.CpuProfile)
	if err := pprof.StartCPUProfile(f); err != nil {
		return empty, err
	}

	go func() {
		config.Logger.Info("Starting pprof server", "addr", config.PprofRpc.Host)
		err := http.ListenAndServe(config.PprofRpc.Host, nil)
		config.Logger.Error("pprof server error", "err", err)
	}()

	return func() {
		config.Logger.Info("stopping CPU profiler", "profile", config.CpuProfile)
		pprof.StopCPUProfile()
		if err := f.Close(); err != nil {
			config.Logger.Info("failed to close cpu-profile file",
				"profile", config.CpuProfile,
				"err", err.Error(),
			)
		}
	}, nil
}
