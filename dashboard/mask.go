package dashboard

import "strings"

// maskHeaders returns a copy of headers with sensitive values replaced by
// "••••••". The comparison is case-insensitive against cfg.MaskedHeaders.
//
// Used by the dashboard middleware before persisting a RequestRecord so
// secrets never enter the rolling buffer.
func maskHeaders(cfg Config, headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	masked := make(map[string]string, len(headers))
	sensitive := make(map[string]struct{}, len(cfg.MaskedHeaders))
	for _, h := range cfg.MaskedHeaders {
		sensitive[strings.ToLower(h)] = struct{}{}
	}
	for k, v := range headers {
		if _, ok := sensitive[strings.ToLower(k)]; ok {
			masked[k] = "••••••"
		} else {
			masked[k] = v
		}
	}
	return masked
}

// maskLine scans s for tokens that look like secrets (key=value, key:value,
// "key":"value") and replaces their value with "••••••". Used by the log
// recorder so application logs that accidentally print tokens are still
// surfaced but redacted.
func maskLine(cfg Config, s string) string {
	if s == "" {
		return s
	}
	sensitive := make(map[string]struct{}, len(cfg.MaskedHeaders))
	for _, h := range cfg.MaskedHeaders {
		sensitive[strings.ToLower(h)] = struct{}{}
	}
	// Simple pass: split on whitespace, for each token check for "key=value"
	// or "key:" prefix matching a sensitive name, then blank the value.
	parts := strings.Fields(s)
	for i, p := range parts {
		lower := strings.ToLower(p)
		for key := range sensitive {
			if strings.HasPrefix(lower, key+"=") {
				parts[i] = key + "=••••••"
				break
			}
			// Match "key:" possibly trailing.
			if strings.HasPrefix(lower, key+":") {
				parts[i] = key + ":••••••"
				break
			}
		}
	}
	return strings.Join(parts, " ")
}
