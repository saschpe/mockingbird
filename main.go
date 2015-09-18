// Mockingbird - Generic HTTP API mocking framework
//
// Copyright 2015 (c) Sascha Peilicke
//

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
	"regexp"
	"strings"
	"time"

	"github.com/justinas/alice"
)

type Endpoint struct {
	Name     string
	Route    string // Human-readable API route (just for the purpose of printing)
	Response []byte
}

// Maps sanitized request hashes to API endpoint mocks
var endpointMap = make(map[string]*Endpoint)

// Everything that contains dates or randomized data...
var httpHeaderBlacklist = []string{
	"user-agent:",
	"date:",
	"if-modified-since:",
	"x-powered-by:",
	"expires:",
	"host:",
	"content-length:",
}

// We'll use this to create a bunch of regexen to match against...
var httpHeaderBlacklistRegexen = make([]*regexp.Regexp, len(httpHeaderBlacklist), len(httpHeaderBlacklist))

func compileHttpHeaderBlacklistRegexen() {
	for i := 0; i < len(httpHeaderBlacklist); i++ {
		s := "(?i)" + httpHeaderBlacklist[i] + " .*\n"
		re := regexp.MustCompile(s)
		log.Printf("New HTTP header filter regexp: %s", re)
		httpHeaderBlacklistRegexen[i] = re
	}
}

// Strips known-bad HTTP headers, return sha1 hash
func hashSanitizedHttpRequest(request string) (string, string) {
	result := request
	for i := 0; i < len(httpHeaderBlacklistRegexen); i++ {
		re := httpHeaderBlacklistRegexen[i]
		result = re.ReplaceAllString(result, "")
	}
	sum := sha1.Sum([]byte(result))
	log.Printf("Sanitized request mock (%x):\n%s", sum, result)
	return fmt.Sprintf("%x", sum), result
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
	hash, _ := hashSanitizedHttpRequest(request)

	log.Printf("Mock route: %s %s (%s)\n", route, name, hash)

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
	incommingHash, cleanRequest := hashSanitizedHttpRequest(string(rawRequest))

	if endpoint, ok := endpointMap[incommingHash]; ok {
		w.Write(endpoint.Response)
		return
	}

	// No endpoint found, return index instead of 404
	fmt.Fprintf(w, "Mockingbird - Generic HTTP API mocking framework\n\n")
	fmt.Fprintf(w, "No mock found for request (%s):\n---\n%s---\n", incommingHash, cleanRequest)
	fmt.Fprintf(w, "Available mocks:\n\n")
	for hash, endpoint := range endpointMap {
		fmt.Fprintf(w, "%s %s (%s)\n", endpoint.Route, endpoint.Name, hash)
	}
}

func main() {
	flag.Parse()

	compileHttpHeaderBlacklistRegexen()

	rootDir := "endpoints" // Our test case backing store
	filepath.Walk(rootDir, discoverTestCases)

	commonHandlers := alice.New(loggingHandler, recoverHandler)
	http.Handle("/", commonHandlers.ThenFunc(mockHandler))
	log.Printf("Initialization done...\n")
	http.ListenAndServe(":8080", nil)
}
