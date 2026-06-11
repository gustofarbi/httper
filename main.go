package main

import (
	"errors"
	"flag"
	"fmt"
	"httper/pkg/env"
	"httper/pkg/request"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var (
	save = flag.Bool(
		"save",
		false,
		"save response to file",
	)
	envFile = flag.String(
		"env-file",
		"",
		"env file to be used to replace placeholders",
	)
	environment = flag.String(
		"env",
		"",
		"env to be used to replace placeholders",
	)
	verbose = flag.Bool(
		"v",
		false,
		"verbose output",
	)
)

// Config holds the runtime options derived from CLI flags.
type Config struct {
	Save    bool
	Verbose bool
}

func main() {
	flag.Parse()

	cfg := Config{Save: *save, Verbose: *verbose}
	initLogger(cfg.Verbose)

	if err := validateInput(); err != nil {
		slog.Error("validating input", "err", err)
		os.Exit(1)
	}

	if err := run(cfg, flag.Arg(0)); err != nil {
		slog.Error("running http", "err", err)
		os.Exit(1)
	}
}

func validateInput() error {
	input := flag.Arg(0)
	if input == "" {
		return errors.New("1st arg must be input file")
	}

	if _, err := os.Stat(input); err != nil {
		return fmt.Errorf("cannot stat file at %s", input)
	}

	if *envFile != "" {
		if _, err := os.Stat(*envFile); err != nil {
			return fmt.Errorf("cannot stat file at %s", *envFile)
		}
	}

	slog.Debug("input file", "name", input)

	return nil
}

func run(cfg Config, input string) error {
	dir := filepath.Dir(input)

	inputRoot, err := os.OpenRoot(dir)
	if err != nil {
		return fmt.Errorf("cannot open root: %w", err)
	}
	defer func() { _ = inputRoot.Close() }()

	// inputRoot is rooted at dir, so open the file by its base name relative to it.
	file, err := inputRoot.OpenFile(filepath.Base(input), os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("cannot open file at %s: %w", input, err)
	}
	defer func() { _ = file.Close() }()

	contentRaw, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("cannot read file at %s: %w", input, err)
	}

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resolve := func(s string) string { return s }
	if *environment != "" {
		envMap := loadEnv(*envFile, *environment)
		if envMap != nil {
			resolve = envMap.Replace
		}
	}

	httpFile, err := request.ParseFile(string(contentRaw))
	if err != nil {
		return fmt.Errorf("cannot parse input file: %w", err)
	}

	if len(httpFile.Templates) == 0 {
		slog.Warn("no requests found in the input file")
		return nil
	}

	// Responses are saved under the current working directory; open the root
	// once and reuse it across every request instead of per-request.
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("cannot get working directory: %w", err)
	}

	saveRoot, err := os.OpenRoot(wd)
	if err != nil {
		return fmt.Errorf("cannot open save root: %w", err)
	}
	defer func() { _ = saveRoot.Close() }()

	runner := &Runner{
		Client:   client,
		Out:      os.Stdout,
		Config:   cfg,
		SaveRoot: saveRoot,
	}

	for i, template := range httpFile.Templates {
		slog.Debug("sending request", "number", i+1, "total", len(httpFile.Templates))

		httpRequest, err := template.Build(resolve, dir)
		if err != nil {
			slog.Error("building request", "err", err, "number", i+1)
			continue
		}

		runner.Send(httpRequest)
	}

	return nil
}

func loadEnv(envFile, environment string) env.Environment {
	if envFile == "" {
		return nil
	}

	root, err := os.OpenRoot(filepath.Dir(envFile))
	if err != nil {
		slog.Error("opening root", "err", err)
		return nil
	}

	// root is rooted at the env file's dir; Parse opens by base name relative to it.
	envs, err := env.Parse(root, filepath.Base(envFile))
	if err != nil {
		slog.Error("parsing env file", "err", err)
		return nil
	}

	return envs[environment]
}

func initLogger(verbose bool) {
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}

	logger := slog.New(
		slog.NewTextHandler(
			os.Stdout, &slog.HandlerOptions{
				Level: logLevel,
			},
		),
	)

	slog.SetDefault(logger)
}
