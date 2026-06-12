package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/gustofarbi/httper/pkg/env"
	"github.com/gustofarbi/httper/pkg/request"
	"github.com/gustofarbi/httper/pkg/script"
	"github.com/gustofarbi/httper/pkg/vars"
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
	vegetaFlag = flag.Bool(
		"vegeta",
		false,
		"run @vegeta-marked requests as load tests",
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

	inputs, err := expandInputs(flag.Args())
	if err != nil {
		slog.Error("validating input", "err", err)
		os.Exit(1)
	}

	report, suites, err := run(cfg, inputs, os.Stdout)
	if err != nil {
		slog.Error("running http", "err", err)
		os.Exit(1)
	}

	if err := writeReportFiles(suites, report, *reportJUnit, *reportJSON); err != nil {
		slog.Error("writing report files", "err", err)
		os.Exit(1)
	}

	// Exit 2 distinguishes "requests ran but tests/sends failed" from usage
	// and I/O errors (exit 1).
	if report.Failed() {
		os.Exit(2)
	}
}

func validateInput() error {
	if len(flag.Args()) == 0 {
		return errors.New("at least one input file is required")
	}

	if *envFile != "" {
		if _, err := os.Stat(*envFile); err != nil {
			return fmt.Errorf("cannot stat file at %s", *envFile)
		}
	}

	return nil
}

// expandInputs glob-expands every argument (a plain existing filename matches
// itself); an argument matching nothing is an error rather than a silent skip.
func expandInputs(args []string) ([]string, error) {
	if len(args) == 0 {
		return nil, errors.New("at least one input file is required")
	}

	var inputs []string
	for _, arg := range args {
		matches, err := filepath.Glob(arg)
		if err != nil {
			return nil, fmt.Errorf("bad pattern %q: %w", arg, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("no files match %q", arg)
		}
		inputs = append(inputs, matches...)
	}

	return inputs, nil
}

// run executes every input file independently and aggregates one report.
// Files are isolated on purpose — fresh cookie jar, globals, and store per
// file — so results never depend on argument or glob order. Shared across
// files: env-file values, -var overrides, and the save root.
func run(cfg Config, inputs []string, out io.Writer) (Report, []Suite, error) {
	envVars := make(map[string]string)
	if *environment != "" {
		for key, value := range loadEnv(*envFile, *environment) {
			envVars[key] = fmt.Sprint(value)
		}
	}

	// Responses are saved under the current working directory; open the root
	// once and reuse it across every request instead of per-request.
	wd, err := os.Getwd()
	if err != nil {
		return Report{}, nil, fmt.Errorf("cannot get working directory: %w", err)
	}

	saveRoot, err := os.OpenRoot(wd)
	if err != nil {
		return Report{}, nil, fmt.Errorf("cannot open save root: %w", err)
	}
	defer func() { _ = saveRoot.Close() }()

	var suites []Suite
	var all []*Result

	for _, input := range inputs {
		if len(inputs) > 1 {
			_, _ = fmt.Fprintf(out, "=== %s\n", input)
		}

		results, err := runFile(cfg, input, envVars, saveRoot, out)
		if err != nil {
			return Report{}, nil, err
		}

		suites = append(suites, Suite{Name: filepath.Base(input), Results: results})
		all = append(all, results...)
	}

	report := buildReport(all, *strict)
	printReport(out, all, report, cfg.Verbose)

	return report, suites, nil
}

// runFile executes one .http file with fresh per-file state.
func runFile(cfg Config, input string, envVars map[string]string, saveRoot *os.Root, out io.Writer) ([]*Result, error) {
	dir := filepath.Dir(input)

	inputRoot, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot open root: %w", err)
	}
	defer func() { _ = inputRoot.Close() }()

	// inputRoot is rooted at dir, so open the file by its base name relative to it.
	file, err := inputRoot.OpenFile(filepath.Base(input), os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("cannot open file at %s: %w", input, err)
	}
	defer func() { _ = file.Close() }()

	contentRaw, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("cannot read file at %s: %w", input, err)
	}

	// Cookie jar on by default so chained requests share cookies (login →
	// authenticated follow-up); `# @no-cookie-jar` opts a request out.
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}

	client := newHTTPClient(jar, cfg.Insecure, time.Duration(*timeout)*time.Second)

	httpFile, err := request.ParseFile(string(contentRaw))
	if err != nil {
		return nil, fmt.Errorf("cannot parse input file: %w", err)
	}

	if len(httpFile.Templates) == 0 {
		slog.Warn("no requests found in the input file", "file", input)
		return nil, nil
	}

	templates, err := filterTemplates(httpFile.Templates, *names)
	if err != nil {
		return nil, err
	}

	globals := vars.NewGlobals()
	store := vars.NewStore(envVars, httpFile.Vars, globals)
	store.SetCLI(cliVars)

	runner := &Runner{
		Client:   client,
		Out:      out,
		Config:   cfg,
		SaveRoot: saveRoot,
	}

	grpcRunner := &GRPCRunner{
		Out:      out,
		Config:   cfg,
		SaveRoot: saveRoot,
		Timeout:  time.Duration(*timeout) * time.Second,
	}

	// nil keeps @vegeta-marked requests running as normal single requests;
	// the -vegeta flag arms the attacks.
	var vegetaRunner *VegetaRunner
	if *vegetaFlag {
		vegetaRunner = &VegetaRunner{
			Out:     out,
			Config:  cfg,
			Timeout: time.Duration(*timeout) * time.Second,
		}
	}

	engine := &script.Engine{Globals: globals, Out: out}

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

	return executeTemplates(runner, grpcRunner, vegetaRunner, templates, store, engine, dir, envVars, loadScript), nil
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
