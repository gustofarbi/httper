package request

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
	"path"
	"strconv"
	"strings"
)

type FormField struct {
	Name        string
	ContentType string
	Headers     textproto.MIMEHeader
	Filename    string
	Content     io.Reader
}

func (f FormField) Encode(w *multipart.Writer) error {
	field, err := w.CreatePart(f.Headers)
	if err != nil {
		return fmt.Errorf("creating form field '%s': %w", f.Name, err)
	}

	if _, err = io.Copy(field, f.Content); err != nil {
		return fmt.Errorf("writing form field '%s': %w", f.Name, err)
	}

	return nil
}

func splitContentType(contentTypeRaw string) (contentType, boundary string) {
	contentType, boundary, _ = strings.Cut(contentTypeRaw, ";")
	contentType = strings.TrimSpace(contentType)

	boundary = strings.TrimSpace(boundary)
	boundary = strings.TrimPrefix(boundary, "boundary=")

	return
}

func getFormDataBody(boundary, body, wd string) (io.Reader, error) {
	fieldsRaw := strings.Split(body, "--"+boundary)

	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)

	if err := writer.SetBoundary(boundary); err != nil {
		return nil, fmt.Errorf("setting boundary: %w", err)
	}

	for i, fieldRaw := range fieldsRaw {
		fieldRaw = strings.TrimSpace(fieldRaw)
		if fieldRaw == "" || fieldRaw == "--" {
			continue
		}
		field, err := parseField(fieldRaw, wd, i)
		if err != nil {
			return nil, fmt.Errorf("parsing form field: %w", err)
		}
		if err = field.Encode(writer); err != nil {
			return nil, fmt.Errorf("encoding form field: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("closing multipart writer: %w", err)
	}

	return buf, nil
}

func parseField(content, wd string, position int) (*FormField, error) {
	headersRaw, bodyRaw, _ := strings.Cut(content, "\n\n")
	headersRaw = strings.TrimPrefix(headersRaw, "--")
	headersRaw = strings.TrimSpace(headersRaw)

	headers := parseHeaders(headersRaw)
	disposition := headers.Get("Content-Disposition")

	result := &FormField{Headers: headers}

	if disposition == "" {
		result.Content = strings.NewReader(bodyRaw)
		result.Name = strconv.Itoa(position)
		return result, nil
	}

	result.Name, result.Filename = getDispositionParts(disposition)

	if result.Filename != "" {
		reader, err := getFiles(bodyRaw, wd)
		if err != nil {
			return nil, fmt.Errorf("getting files: %w", err)
		}

		result.Content = reader
	} else {
		result.Content = strings.NewReader(bodyRaw)

	}

	return result, nil
}

func getFiles(bodyRaw, wd string) (io.Reader, error) {
	lines := strings.Split(bodyRaw, "\n")
	files := make([]io.Reader, 0, len(lines))

	root, err := os.OpenRoot(wd)
	if err != nil {
		return nil, fmt.Errorf("opening root: %w", err)
	}

	for i, line := range lines {
		if !strings.HasPrefix(line, "< ") {
			continue
		}

		filename := strings.TrimPrefix(line, "< ")
		filename = strings.TrimSpace(filename)
		if filename == "" {
			continue
		}

		// root is rooted at wd, so OpenFile needs the path relative to it.
		f, err := root.OpenFile(filename, os.O_RDONLY, 0)
		if err != nil {
			return nil, fmt.Errorf("opening file %s: %w", path.Join(wd, filename), err)
		}

		files = append(files, f)

		if i != len(lines)-1 {
			files = append(files, strings.NewReader("\n"))
		}
	}

	if len(files) == 0 {
		return strings.NewReader(""), nil
	}

	return io.MultiReader(files...), nil
}

func getDispositionParts(disposition string) (name, filename string) {
	dispositionParts := strings.Split(disposition, ";")
	for _, part := range dispositionParts {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "name=") {
			name = strings.TrimPrefix(part, "name=")
			name = strings.Trim(name, "\"")
		} else if strings.HasPrefix(part, "filename=") {
			filename = strings.TrimPrefix(part, "filename=")
			filename = strings.Trim(filename, "\"")
		}
	}

	return
}
