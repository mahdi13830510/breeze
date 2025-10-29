package breeze

import (
	"bytes"
	"fmt"
)

// Bytes converts the HTTPResponse to raw HTTP bytes for sending.
func (r *HTTPResponse) Bytes() []byte {
	var b bytes.Buffer
	statusText := map[int]string{
		200: "OK",
		400: "Bad Request",
		404: "Not Found",
		500: "Internal Server Error",
	}[r.Status]
	if statusText == "" {
		statusText = "OK"
	}

	b.WriteString(fmt.Sprintf("HTTP/1.1 %d %s\r\n", r.Status, statusText))
	for k, v := range r.Headers {
		b.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	b.WriteString(fmt.Sprintf("Content-Length: %d\r\n\r\n", len(r.Body)))
	b.Write(r.Body)
	return b.Bytes()
}
