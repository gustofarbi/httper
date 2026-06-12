package request

// IsGRPC reports whether the request line's method is GRPC.
func (t *Template) IsGRPC() bool {
	method, _, _ := SplitEssentials(t.Essentials)

	return method == "GRPC"
}

// BuildGRPC resolves the template's sections for a gRPC send: the raw target
// URL, the header pairs (future metadata), and the JSON body. Like Build,
// resolution happens here at send time so request chaining works.
func (t *Template) BuildGRPC(resolve func(string) string) (rawURL string, headers [][2]string, body string) {
	_, rawURL, _ = SplitEssentials(resolve(t.Essentials))

	return rawURL, HeaderPairs(resolve(t.HeadersRaw)), resolve(t.BodyRaw)
}
