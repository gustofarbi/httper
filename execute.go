package main

import (
	"httper/pkg/request"
	"httper/pkg/script"
	"httper/pkg/vars"
	"log/slog"
)

// executeTemplates runs each template through the full per-request pipeline:
// pre-request script → placeholder resolution → build → send → response
// handler script. envVars backs request.environment in pre-request scripts;
// loadScript reads `> file.js` handler sources; both may be nil.
func executeTemplates(
	runner *Runner,
	grpcRunner *GRPCRunner,
	templates []*request.Template,
	store *vars.Store,
	engine *script.Engine,
	wd string,
	envVars map[string]string,
	loadScript func(path string) (string, error),
) []*Result {
	results := make([]*Result, 0, len(templates))

	for i, template := range templates {
		slog.Debug("sending request", "number", i+1, "total", len(templates))

		store.ClearLocal()

		if template.PreScript.Code != "" {
			if err := engine.RunPre(template.PreScript.Code, preRequestFor(template, store, envVars), store.SetLocal); err != nil {
				slog.Error("pre-request script", "err", err, "request", template.Name)
				results = append(results, &Result{Name: template.Name, Err: err})
				continue
			}
		}

		var result *Result
		if template.IsGRPC() {
			result = grpcRunner.Send(template, store.Resolve)
		} else {
			httpRequest, err := template.Build(store.Resolve, wd)
			if err != nil {
				slog.Error("building request", "err", err, "request", template.Name)
				results = append(results, &Result{Name: template.Name, Err: err})
				continue
			}
			result = runner.Send(template, httpRequest)
		}
		results = append(results, result)
		if result.Err != nil {
			continue
		}

		code, err := handlerSource(template.PostScript, loadScript)
		if err != nil {
			slog.Error("loading response handler", "err", err, "request", template.Name)
			result.Err = err
			continue
		}
		if code == "" {
			continue
		}

		contentType := result.Header.Get("Content-Type")
		if result.GRPC {
			// gRPC bodies are always rendered as JSON; synthesize the type so
			// response.body parses.
			contentType = "application/json"
		}

		tests, err := engine.RunPost(code, &script.Response{
			Status:      result.StatusCode,
			Headers:     result.Header,
			ContentType: contentType,
			Body:        result.Body,
		})
		result.Tests = tests
		if err != nil {
			slog.Error("response handler script", "err", err, "request", template.Name)
			result.Err = err
		}
	}

	return results
}

// preRequestFor builds the raw request view a pre-request script sees:
// unresolved essentials/headers/body plus the env-file values, with
// store.Resolve backing the tryGetSubstituted accessors.
func preRequestFor(template *request.Template, store *vars.Store, envVars map[string]string) *script.PreRequest {
	method, rawURL, _ := request.SplitEssentials(template.Essentials)

	return &script.PreRequest{
		Method:      method,
		URL:         rawURL,
		Body:        template.BodyRaw,
		Headers:     request.HeaderPairs(template.HeadersRaw),
		Environment: envVars,
		Resolve:     store.Resolve,
	}
}

func handlerSource(s request.Script, loadScript func(path string) (string, error)) (string, error) {
	if s.Code != "" || s.Path == "" {
		return s.Code, nil
	}
	if loadScript == nil {
		return "", nil
	}

	return loadScript(s.Path)
}
