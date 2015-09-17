// Mockingbird - Generic HTTP API mocking framework
//
// Copyright 2015 (c) Sascha Peilicke
//
// Has a 'database' of request / response pairs which you are free to call 'test
// cases'. Real requests against the API mock are matched against those cases:
//
// All volatile HTTP headers are stripped (like User-Agent, Date,
// If-Modified-Since, ...). Sanitized requests are then hashed and compared
// against our (equally sanitized on hashed) list of request test cases. If a
// match is found the corresponding respond is returned.
//
// In other words, this API endpoint mocking framework is totally agnostic of
// the payload. You can deliver SOAP, REST or binary blob test cases as long as
// your API uses HTTP request / response objects.

package main

import (
	"crypto/sha1"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/justinas/alice"
)

type Endpoint struct {
	Name     string
	Route    string // Human-readable API route (just for the purpose of printing)
	Response []byte
}

// Everything that contains dates or randomized data...
var httpHeaderBlacklist = []string{
	"user-agent:",
	"date:",
	"if-modified-since:",
	"x-powered-by:",
	"expires:",
	"host:",
	"user-agent:",
	"content-length:",
}

func containsBlacklistedHeader(text string) bool {
	lowerText := strings.ToLower(text)

	// Building up a prefix tree would be better, but this is Q'n'D :-)
	for i := 0; i < len(httpHeaderBlacklist); i++ {
		if strings.Contains(lowerText, httpHeaderBlacklist[i]) {
			return true
		}
	}
	return false
}

// Maps sanitized request hashes to API endpoint mocks
var endpointMap = make(map[string]*Endpoint)

// Strips HTTP body and known-bad HTTP headers, return sha1 hash
func hashSanitizedHttpRequest(request string) string {
	h := sha1.New()
	lines := strings.Split(request, "\n")
	log.Printf("Sanitizing request:\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if !containsBlacklistedHeader(line) {
			// Header not blacklisted, append to buffer...
			h.Write([]byte(line))
			h.Write([]byte("\n"))
			fmt.Printf("%s\n", line)
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}

// Loads a request / response test case pair from our backing store
func loadTestCase(requestFile string, responseFile string) {
	requestBytes, _ := ioutil.ReadFile(requestFile) // Files are not too large...
	responseBytes, _ := ioutil.ReadFile(responseFile)
	request := string(requestBytes)

	// Retrieve HTTP request route by splitting twice and return the 2nd element
	// e.g. "GET /index.html HTTP/1.1" will yield "/index.html". That's all we need
	route := strings.SplitN(request, " ", 3)[1]
	name := strings.TrimSuffix(path.Base(requestFile), ".request")
	hash := hashSanitizedHttpRequest(request)

	log.Printf("Mock route: %s case: %s (hash: %s)\n", route, name, hash)

	endpointMap[hash] = &Endpoint{Route: route, Name: name, Response: responseBytes}
}

var currentRequestFile string // TODO: Hide in closure
// Discovers request / response test cases
func discoverTestCases(path string, f os.FileInfo, err error) error {
	if strings.HasSuffix(path, ".request") {
		currentRequestFile = path
	} else if strings.HasSuffix(path, ".response") {
		trimmedRequestFile := strings.TrimSuffix(currentRequestFile, ".request")
		trimmedResponseFile := strings.TrimSuffix(path, ".response")

		// If both base paths (without suffix) match, we do have a request response pair
		if strings.Compare(trimmedRequestFile, trimmedResponseFile) == 0 {
			loadTestCase(currentRequestFile, path)
		}
	}

	return nil
}

// Logging middleware
func loggingHandler(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		t1 := time.Now()
		next.ServeHTTP(w, r)
		t2 := time.Now()
		log.Printf("[%s] %q %v\n", r.Method, r.URL.String(), t2.Sub(t1))
	}

	return http.HandlerFunc(fn)
}

// Recovery middleware
func recoverHandler(next http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("panic: %+v", err)
				http.Error(w, http.StatusText(500), 500)
			}
		}()

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(fn)
}

func mockHandler(w http.ResponseWriter, r *http.Request) {
	rawRequest, _ := httputil.DumpRequest(r, true)
	incommingHash := hashSanitizedHttpRequest(string(rawRequest))

	log.Printf("Incomming hash: %s", incommingHash)

	if endpoint, ok := endpointMap[incommingHash]; ok {
		w.Write(endpoint.Response)
		return
	}

	// No endpoint found, return index instead of 404
	fmt.Fprintf(w, "Mockingbird - Generic HTTP API mocking framework\n\n")
	fmt.Fprintf(w, "No mock found for request (%s): \n\n%s\n\n", incommingHash, rawRequest)
	fmt.Fprintf(w, "Available mocks:\n\n")
	for hash, endpoint := range endpointMap {
		fmt.Fprintf(w, "%s case: %s (hash: %s)\n", endpoint.Route, endpoint.Name, hash)
	}
}

func main() {
	flag.Parse()

	rootDir := "endpoints" // Our test case backing store
	filepath.Walk(rootDir, discoverTestCases)

	commonHandlers := alice.New(loggingHandler, recoverHandler)
	http.Handle("/", commonHandlers.ThenFunc(mockHandler))
	http.ListenAndServe(":8080", nil)
}
