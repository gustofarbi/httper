package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Suite groups one input file's results for the report writers; multi-file
// runs produce one suite per file.
type Suite struct {
	Name    string
	Results []*Result
}

// writeReportFiles writes the JUnit and/or JSON reports; empty paths are
// skipped. Files are created through os.Root rooted at their directory,
// matching the sandboxing convention everywhere else in the tool.
func writeReportFiles(suites []Suite, report Report, junitPath, jsonPath string) error {
	write := func(path string, render func(io.Writer) error) error {
		if path == "" {
			return nil
		}

		root, err := os.OpenRoot(filepath.Dir(path))
		if err != nil {
			return fmt.Errorf("opening report dir: %w", err)
		}
		defer func() { _ = root.Close() }()

		file, err := root.OpenFile(filepath.Base(path), os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
		if err != nil {
			return fmt.Errorf("creating report file %s: %w", path, err)
		}
		defer func() { _ = file.Close() }()

		return render(file)
	}

	if err := write(junitPath, func(w io.Writer) error { return writeJUnit(w, suites) }); err != nil {
		return err
	}

	return write(jsonPath, func(w io.Writer) error { return writeJSONReport(w, suites, report) })
}

type junitTestsuites struct {
	XMLName  xml.Name         `xml:"testsuites"`
	Tests    int              `xml:"tests,attr"`
	Failures int              `xml:"failures,attr"`
	Errors   int              `xml:"errors,attr"`
	Suites   []junitTestsuite `xml:"testsuite"`
}

type junitTestsuite struct {
	Name     string          `xml:"name,attr"`
	Tests    int             `xml:"tests,attr"`
	Failures int             `xml:"failures,attr"`
	Errors   int             `xml:"errors,attr"`
	Cases    []junitTestcase `xml:"testcase"`
}

type junitTestcase struct {
	Name      string        `xml:"name,attr"`
	Classname string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitMessage `xml:"failure,omitempty"`
	Error     *junitMessage `xml:"error,omitempty"`
}

type junitMessage struct {
	Message string `xml:"message,attr"`
}

// writeJUnit renders suites as JUnit XML: one testcase per client.test, an
// error testcase for a failed send, and one passing testcase for a request
// without tests so CI still shows it ran.
func writeJUnit(w io.Writer, suites []Suite) error {
	doc := junitTestsuites{}

	for _, suite := range suites {
		js := junitTestsuite{Name: suite.Name}

		for _, result := range suite.Results {
			duration := fmt.Sprintf("%.3f", result.Duration.Seconds())

			switch {
			case result.Err != nil:
				js.Errors++
				js.Cases = append(js.Cases, junitTestcase{
					Name:      result.Name,
					Classname: suite.Name,
					Time:      duration,
					Error:     &junitMessage{Message: result.Err.Error()},
				})
			case len(result.Tests) == 0:
				js.Cases = append(js.Cases, junitTestcase{
					Name:      result.Name,
					Classname: suite.Name,
					Time:      duration,
				})
			default:
				for _, test := range result.Tests {
					c := junitTestcase{
						Name:      result.Name + " / " + test.Name,
						Classname: suite.Name,
						Time:      duration,
					}
					if test.Failed {
						js.Failures++
						c.Failure = &junitMessage{Message: test.Message}
					}
					js.Cases = append(js.Cases, c)
				}
			}
		}

		js.Tests = len(js.Cases)
		doc.Tests += js.Tests
		doc.Failures += js.Failures
		doc.Errors += js.Errors
		doc.Suites = append(doc.Suites, js)
	}

	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}

	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		return fmt.Errorf("encoding junit report: %w", err)
	}

	_, err := io.WriteString(w, "\n")
	return err
}
