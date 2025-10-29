package breeze

import (
	"bytes"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ParseHTTPRequest parses raw bytes into an HTTPRequest.
func ParseHTTPRequest(data []byte) (*HTTPRequest, int, error) {
	headerEnd := bytes.Index(data, []byte("\r\n\r\n"))
	if headerEnd < 0 {
		return nil, 0, nil
	}

	lines := bytes.Split(data[:headerEnd], []byte("\r\n"))
	if len(lines) < 1 {
		return nil, 0, nil
	}

	parts := strings.SplitN(string(lines[0]), " ", 3)
	if len(parts) < 2 {
		return nil, 0, fmt.Errorf("malformed request line")
	}

	methodStr, rawPath := parts[0], parts[1]
	u, err := url.Parse(rawPath)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid path: %w", err)
	}

	req := &HTTPRequest{
		Method: Method(methodStr),
		Path:   u.Path,
		Query:  u.Query(),
		Header: make(map[string]string),
	}

	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if len(line) == 0 {
			continue
		}
		colon := bytes.IndexByte(line, ':')
		if colon > 0 {
			key := strings.ToLower(strings.TrimSpace(string(line[:colon])))
			value := strings.TrimSpace(string(line[colon+1:]))
			req.Header[key] = value
		}
	}

	clStr := req.Header["content-length"]
	if clStr != "" {
		cl, err := strconv.Atoi(clStr)
		if err != nil || cl < 0 {
			return nil, 0, fmt.Errorf("invalid content-length")
		}
		total := headerEnd + 4 + cl
		if len(data) < total {
			return nil, 0, nil
		}
		req.Body = data[headerEnd+4 : total]
		return req, total, nil
	}

	return req, headerEnd + 4, nil
}
