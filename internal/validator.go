package internal

// // URL validates if the given string is a valid URL.
// func URL(rawURL string) bool {
// 	if strings.TrimSpace(rawURL) == "" {
// 		return false
// 	}

// 	u, err := url.Parse(rawURL)
// 	if err != nil {
// 		return false
// 	}

// 	return u.Scheme != "" && u.Host != ""
// }

// // AbsoluteURL validates if the given string is an absolute URL.
// func AbsoluteURL(rawURL string) bool {
// 	if !URL(rawURL) {
// 		return false
// 	}

// 	u, _ := url.Parse(rawURL) // Error already checked in URL()
// 	return u.IsAbs()
// }

// // HTTPSUrl validates if the given string is a valid HTTPS URL.
// func HTTPSUrl(rawURL string) bool {
// 	if !URL(rawURL) {
// 		return false
// 	}

// 	u, _ := url.Parse(rawURL) // Error already checked in URL()
// 	return strings.ToLower(u.Scheme) == "https"
// }

// // SafeWebURL validates if the URL is safe for web applications.
// func SafeWebURL(rawURL string) bool {
// 	if !URL(rawURL) {
// 		return false
// 	}

// 	u, _ := url.Parse(rawURL) // Error already checked in URL()

// 	// Only allow HTTP/HTTPS schemes
// 	scheme := strings.ToLower(u.Scheme)
// 	if scheme != "http" && scheme != "https" {
// 		return false
// 	}

// 	// Optional: Check URL length
// 	if len(rawURL) > 2048 { // Common browser URL length limit
// 		return false
// 	}

// 	// Optional: Check for private/local addresses
// 	host := strings.ToLower(u.Host)
// 	if strings.Contains(host, "localhost") ||
// 		strings.Contains(host, "127.0.0.1") ||
// 		strings.Contains(host, "::1") {
// 		return false
// 	}

// 	return true
// }

// // Endpoint validates if the given string is a valid API endpoint URL.
// func Endpoint(rawURL string) bool {
// 	// Reject empty or whitespace-only strings
// 	if strings.TrimSpace(rawURL) == "" {
// 		return false
// 	}

// 	u, err := url.Parse(rawURL)
// 	if err != nil {
// 		return false
// 	}

// 	// Must have scheme and host
// 	if u.Scheme == "" || u.Host == "" {
// 		return false
// 	}

// 	// Only allow HTTP/HTTPS schemes
// 	scheme := strings.ToLower(u.Scheme)
// 	if scheme != "http" && scheme != "https" {
// 		return false
// 	}

// 	// Reject URLs with fragments (#)
// 	if u.Fragment != "" {
// 		return false
// 	}

// 	// Reject URLs with user info
// 	if u.User != nil {
// 		return false
// 	}

// 	// Optional: Reject localhost and private IPs for production
// 	host := strings.ToLower(u.Host)
// 	if strings.Contains(host, "localhost") ||
// 		strings.Contains(host, "127.0.0.1") ||
// 		strings.Contains(host, "::1") {
// 		return false
// 	}

// 	// Optional: Check URL length (adjust limit as needed)
// 	if len(rawURL) > 2048 {
// 		return false
// 	}

// 	return true
// }

// // EndpointWithOptions validates endpoint URL with custom options.
// func EndpointWithOptions(rawURL string, opts Options) bool {
// 	if !Endpoint(rawURL) {
// 		return false
// 	}

// 	u, _ := url.Parse(rawURL) // Error already checked in Endpoint()

// 	// Check allowed schemes
// 	if len(opts.AllowedSchemes) > 0 {
// 		schemeOK := false
// 		for _, scheme := range opts.AllowedSchemes {
// 			if strings.ToLower(u.Scheme) == strings.ToLower(scheme) {
// 				schemeOK = true
// 				break
// 			}
// 		}
// 		if !schemeOK {
// 			return false
// 		}
// 	}

// 	// Check allowed hosts
// 	if len(opts.AllowedHosts) > 0 {
// 		hostOK := false
// 		for _, host := range opts.AllowedHosts {
// 			if strings.ToLower(u.Host) == strings.ToLower(host) {
// 				hostOK = true
// 				break
// 			}
// 		}
// 		if !hostOK {
// 			return false
// 		}
// 	}

// 	// Check max length
// 	if opts.MaxLength > 0 && len(rawURL) > opts.MaxLength {
// 		return false
// 	}

// 	// Allow localhost/private IPs if specified
// 	if !opts.AllowLocalhost {
// 		host := strings.ToLower(u.Host)
// 		if strings.Contains(host, "localhost") ||
// 			strings.Contains(host, "127.0.0.1") ||
// 			strings.Contains(host, "::1") {
// 			return false
// 		}
// 	}

// 	return true
// }

// // Options configures endpoint validation rules.
// type Options struct {
// 	AllowedSchemes []string // List of allowed schemes (e.g., ["https"])
// 	AllowedHosts   []string // List of allowed hosts
// 	MaxLength      int      // Maximum URL length (0 for no limit)
// 	AllowLocalhost bool     // Allow localhost and private IPs
// }
