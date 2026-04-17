package sourcemapper

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// sourceMap represents a sourceMap. We only really care about the sources and
// sourcesContent arrays.
type SourceMap struct {
	Version        int      `json:"version"`
	Sources        []string `json:"sources"`
	SourcesContent []string `json:"sourcesContent"`
}

// command line args
type Config struct {
	Outdir string // output directory
	Url    string // sourcemap url
	Jsurl  string // javascript url
}

// getSourceMap retrieves a sourcemap from a URL or a local file and returns
// its sourceMap.
func GetSourceMap(source string) (m SourceMap, err error) {
	var body []byte
	var client http.Client

	log.Printf("[+] Retrieving Sourcemap from %.1024s...\n", source)

	u, err := url.ParseRequestURI(source)
	if err != nil {
		// If it's a file, read it.
		body, err = os.ReadFile(source)
		if err != nil {
			log.Fatalln(err)
		}
	} else {
		if u.Scheme == "http" || u.Scheme == "https" {
			// If it's a URL, get it.
			req, err := http.NewRequest("GET", u.String(), nil)
			tr := &http.Transport{}
			tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

			if err != nil {
				log.Fatalln(err)
			}

			client = http.Client{
				Transport: tr,
			}

			res, err := client.Do(req)

			if err != nil {
				log.Fatalln(err)
			}

			body, err = io.ReadAll(res.Body)
			defer res.Body.Close()

			if res.StatusCode != 200 && len(body) > 0 {
				log.Printf("[!] WARNING - non-200 status code: %d - Confirm this URL contains valid source map manually!", res.StatusCode)
				log.Printf("[!] WARNING - sourceMap URL request return != 200 - however, body length > 0 so continuing... ")
			}

			if err != nil {
				log.Fatalln(err)
			}
		} else if u.Scheme == "data" {
			urlchunks := strings.Split(u.Opaque, ",")
			if len(urlchunks) < 2 {
				log.Fatalf("[!] Could not parse data URI - expected atleast 2 chunks but got %d\n", len(urlchunks))
			}

			data, err := base64.StdEncoding.DecodeString(urlchunks[1])
			if err != nil {
				log.Fatal("[!] Error base64 decoding", err)
			}

			body = []byte(data)
		} else {
			// If it's a file, read it.
			body, err = os.ReadFile(source)
			if err != nil {
				log.Fatalln(err)
			}
		}
	}
	// Unmarshall the body into the struct.
	log.Printf("[+] Read %d bytes, parsing JSON.\n", len(body))
	err = json.Unmarshal(body, &m)

	if err != nil {
		log.Printf("[!] Error parsing JSON - confirm %s is a valid JS sourcemap", source)
	}

	return
}

// getSourceMapFromJS queries a JavaScript URL, parses its headers and content and looks for sourcemaps
// follows the rules outlined in https://tc39.es/source-map-spec/#linking-generated-code
func GetSourceMapFromJS(jsurl string) (m SourceMap, err error) {
	var client http.Client

	log.Printf("[+] Retrieving JavaScript from URL: %s.\n", jsurl)

	// perform the request
	u, err := url.ParseRequestURI(jsurl)
	if err != nil {
		log.Fatal(err)
	}

	req, err := http.NewRequest("GET", u.String(), nil)
	tr := &http.Transport{}
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	if err != nil {
		log.Fatalln(err)
	}

	client = http.Client{
		Transport: tr,
	}

	res, err := client.Do(req)

	if err != nil {
		log.Fatalln(err)
	}

	if res.StatusCode != 200 {
		log.Fatalf("[!] non-200 status code: %d", res.StatusCode)
	}

	var sourceMap string

	// check for SourceMap and X-SourceMap (deprecated) headers
	if sourceMap = res.Header.Get("SourceMap"); sourceMap == "" {
		sourceMap = res.Header.Get("X-SourceMap")
	}

	if sourceMap != "" {
		log.Printf("[.] Found SourceMap URI in response headers: %.1024s...", sourceMap)
	} else {
		// parse the javascript
		body, err := io.ReadAll(res.Body)
		if err != nil {
			log.Fatalln(err)
		}
		defer res.Body.Close()

		// JS file can have multiple source maps in it, but only the last line is valid https://sourcemaps.info/spec.html#h.lmz475t4mvbx
		re := regexp.MustCompile(`\/\/[@#] sourceMappingURL=(.*)`)
		match := re.FindAllSubmatch(body, -1)

		if len(match) != 0 {
			// only the sourcemap at the end of the file should be valid
			sourceMap = string(match[len(match)-1][1])
			log.Printf("[.] Found SourceMap in JavaScript body: %.1024s...", sourceMap)
		}
	}

	// this introduces a forced request bug if the JS file we're parsing is
	// malicious and forces us to make a request out to something dodgy - take care
	if sourceMap != "" {
		var sourceMapURL *url.URL
		// handle absolute/relative rules
		sourceMapURL, err = url.ParseRequestURI(sourceMap)
		if err != nil {
			// relative url...
			sourceMapURL, err = u.Parse(sourceMap)
			if err != nil {
				log.Fatal(err)
			}
		}

		return GetSourceMap(sourceMapURL.String())
	}

	err = errors.New("[!] No sourcemap URL found")
	return
}

// writeFile writes content to file at path p.
func WriteFile(p string, content string) error {
	p = filepath.Clean(p)

	if _, err := os.Stat(filepath.Dir(p)); os.IsNotExist(err) {
		// Using MkdirAll here is tricky, because even if we fail, we might have
		// created some of the parent directories.
		err = os.MkdirAll(filepath.Dir(p), 0700)
		if err != nil {
			return err
		}
	}

	log.Printf("[+] Writing %d bytes to %s.\n", len(content), p)
	return os.WriteFile(p, []byte(content), 0600)
}

// cleanWindows replaces the illegal characters from a path with `-`.
func CleanWindows(p string) string {
	m1 := regexp.MustCompile(`[?%*|:"<>]`)
	return m1.ReplaceAllString(p, "")
}
