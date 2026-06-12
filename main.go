package main

import (
	"errors"
	"flag"
	"fmt"
	"httper/pkg/env"
	"httper/pkg/request"
	"httper/pkg/script"
	"httper/pkg/vars"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// version is overridden at release build time via
// -ldflags "-X main.version=...".
var version = "dev"

var (
	showVersion = flag.Bool(
		"version",
		false,
		"print version and exit",
	)
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
	names = flag.String(
		"name",
		"",
		"run only requests with these names (comma-separated)",
	)
	strict = flag.Bool(
		"strict",
		false,
		"treat non-2xx responses as failures",
	)
	insecure = flag.Bool(
		"insecure",
		false,
		"skip TLS certificate verification",
	)
	timeout = flag.Int(
		"timeout",
		30,
		"request timeout in seconds (0 disables; per-request @timeout wins)",
	)
	reportJUnit = flag.String(
		"report-junit",
		"",
		"write a JUnit XML report to this path",
	)
	reportJSON = flag.String(
		"report-json",
		"",
		"write a JSON report to this path",
	)
	cliVars = make(varFlags)
)

func init() {
	flag.Var(cliVars, "var", "set a variable as key=value (repeatable; overrides @vars and env file)")
}

// varFlags collects repeatable -var key=value flags.
type varFlags map[string]string

func (v varFlags) String() string {
	return fmt.Sprint(map[string]string(v))
}

func (v varFlags) Set(s string) error {
	key, value, ok := strings.Cut(s, "=")
	if !ok || key == "" {
		return fmt.Errorf("expected key=value, got %q", s)
	}

	v[key] = value
	return nil
}

// Config holds the runtime options derived from CLI flags.
type Config struct {
	Save     bool
	Verbose  bool
	Insecure bool
}

func versionString() string {
	return "httper " + version
}

func main() {
	flag.Parse()

	if *showVersion {
		fmt.Println(versionString())
		return
	}

	cfg := Config{Save: *save, Verbose: *verbose, Insecure: *insecure}
	initLogger(cfg.Verbose)

	if err := validateInput(); err != nil {
		slog.Error("validating input", "err", err)
		os.Exit(1)
	}

	report, err := run(cfg, flag.Arg(0))
	if err != nil {
		slog.Error("running http", "err", err)
		os.Exit(1)
	}

	// Exit 2 distinguishes "requests ran but tests/sends failed" from usage
	// and I/O errors (exit 1).
	if report.Failed() {
		os.Exit(2)
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

func run(cfg Config, input string) (Report, error) {
	dir := filepath.Dir(input)

	inputRoot, err := os.OpenRoot(dir)
	if err != nil {
		return Report{}, fmt.Errorf("cannot open root: %w", err)
	}
	defer func() { _ = inputRoot.Close() }()

	// inputRoot is rooted at dir, so open the file by its base name relative to it.
	file, err := inputRoot.OpenFile(filepath.Base(input), os.O_RDONLY, 0)
	if err != nil {
		return Report{}, fmt.Errorf("cannot open file at %s: %w", input, err)
	}
	defer func() { _ = file.Close() }()

	contentRaw, err := io.ReadAll(file)
	if err != nil {
		return Report{}, fmt.Errorf("cannot read file at %s: %w", input, err)
	}

	// Cookie jar on by default so chained requests share cookies (login →
	// authenticated follow-up); `# @no-cookie-jar` opts a request out.
	jar, err := cookiejar.New(nil)
	if err != nil {
		return Report{}, fmt.Errorf("creating cookie jar: %w", err)
	}

	client := newHTTPClient(jar, cfg.Insecure, time.Duration(*timeout)*time.Second)

	envVars := make(map[string]string)
	if *environment != "" {
		for key, value := range loadEnv(*envFile, *environment) {
			envVars[key] = fmt.Sprint(value)
		}
	}

	httpFile, err := request.ParseFile(string(contentRaw))
	if err != nil {
		return Report{}, fmt.Errorf("cannot parse input file: %w", err)
	}

	templates, err := filterTemplates(httpFile.Templates, *names)
	if err != nil {
		return Report{}, err
	}

	globals := vars.NewGlobals()
	store := vars.NewStore(envVars, httpFile.Vars, globals)
	store.SetCLI(cliVars)

	if len(httpFile.Templates) == 0 {
		slog.Warn("no requests found in the input file")
		return Report{}, nil
	}

	// Responses are saved under the current working directory; open the root
	// once and reuse it across every request instead of per-request.
	wd, err := os.Getwd()
	if err != nil {
		return Report{}, fmt.Errorf("cannot get working directory: %w", err)
	}

	saveRoot, err := os.OpenRoot(wd)
	if err != nil {
		return Report{}, fmt.Errorf("cannot open save root: %w", err)
	}
	defer func() { _ = saveRoot.Close() }()

	runner := &Runner{
		Client:   client,
		Out:      os.Stdout,
		Config:   cfg,
		SaveRoot: saveRoot,
	}

	engine := &script.Engine{Globals: globals, Out: os.Stdout}

	loadScript := func(path string) (string, error) {
		// inputRoot is rooted at the .http file's dir, so handler script paths
		// stay sandboxed there.
		f, err := inputRoot.Open(path)
		if err != nil {
			return "", fmt.Errorf("opening handler script %s: %w", path, err)
		}
		defer func() { _ = f.Close() }()

		code, err := io.ReadAll(f)
		if err != nil {
			return "", fmt.Errorf("reading handler script %s: %w", path, err)
		}

		return string(code), nil
	}

	results := executeTemplates(runner, templates, store, engine, dir, envVars, loadScript)

	report := buildReport(results, *strict)
	printReport(os.Stdout, results, report, cfg.Verbose)

	suites := []Suite{{Name: filepath.Base(input), Results: results}}
	if err := writeReportFiles(suites, report, *reportJUnit, *reportJSON); err != nil {
		return Report{}, fmt.Errorf("writing report files: %w", err)
	}

	return report, nil
}

// newHTTPClient builds the base client; insecure swaps in a cloned transport
// that skips TLS verification (the h2 prior-knowledge transport gets the same
// treatment in Runner.clientFor).
func newHTTPClient(jar http.CookieJar, insecure bool, timeout time.Duration) *http.Client {
	client := &http.Client{
		Timeout: timeout,
		Jar:     jar,
	}

	if insecure {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = insecureTLSConfig(true)
		client.Transport = transport
	}

	return client
}

// filterTemplates keeps only templates whose name is in the comma-separated
// filter, preserving file order. An empty filter keeps everything.
func filterTemplates(templates []*request.Template, filter string) ([]*request.Template, error) {
	if filter == "" {
		return templates, nil
	}

	wanted := make(map[string]bool)
	for _, name := range strings.Split(filter, ",") {
		wanted[strings.TrimSpace(name)] = true
	}

	kept := make([]*request.Template, 0, len(wanted))
	for _, template := range templates {
		if wanted[template.Name] {
			kept = append(kept, template)
		}
	}

	if len(kept) == 0 {
		return nil, fmt.Errorf("no requests match -name %q", filter)
	}

	return kept, nil
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
	defer func() { _ = root.Close() }()

	// root is rooted at the env file's dir; Parse opens by base name relative to it.
	envs, err := env.Parse(root, filepath.Base(envFile))
	if err != nil {
		slog.Error("parsing env file", "err", err)
		return nil
	}

	// JetBrains convention: a private sibling file overlays the public one,
	// key-wise per environment. Missing private file is the normal case.
	if privateName := privateEnvName(filepath.Base(envFile)); privateName != "" {
		private, err := env.Parse(root, privateName)
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			slog.Error("parsing private env file", "err", err)
		default:
			envs = env.Merge(envs, private)
		}
	}

	return envs[environment]
}

// privateEnvName derives the private sibling of an env file name:
// `*env.json` gains a `private.` segment (http-client.env.json →
// http-client.private.env.json), other .json files get `.private` before the
// extension. Returns "" when the name is already private or not derivable.
func privateEnvName(base string) string {
	if strings.Contains(base, "private") {
		return ""
	}

	if rest, ok := strings.CutSuffix(base, "env.json"); ok {
		return rest + "private.env.json"
	}
	if rest, ok := strings.CutSuffix(base, ".json"); ok {
		return rest + ".private.json"
	}

	return ""
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
