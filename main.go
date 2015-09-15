//
// Generic HTTP API Endpoint mocking framework
//
// Copyright (c) Sascha Peilicke
//
// Has a 'database' of request / response pairs which you are free to call 'test
// cases'. Real requests against the API mock are matched against those cases in
// a clever way:
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
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/justinas/alice"
)

type Endpoint struct {
	Route    string // Human-readable API route (just for the purpose of printing)
	Name     string
	Request  []byte
	Response []byte
}

// Everything that contains dates or randomized data...
var httpHeaderBlacklist = map[string]int{
	"user-agent":        1,
	"date":              1,
	"if-modified-since": 1,
	"x-powered-by":      1,
	"expires":           1,
}

// Maps sanitized request hashes to API endpoint mocks
var testSuite map[string]Endpoint

// Strips HTTP body and known-bad HTTP headers, return sha1 hash
func hashHttpHeaders(httpAnything string) []byte {
	hash := sha1.New()

	// Kill HTTP body and have a nice header array left...
	headerList := strings.Split(strings.SplitN(httpAnything, "\n\n", 2)[0], "\n")
	for i := 0; i < len(headerList); i++ {
		header := headerList[i]
		if _, ok := httpHeaderBlacklist[header]; !ok {
			// Header not blacklisted, append to buffer...
			io.WriteString(hash, header)
		}
	}

	return hash.Sum(nil)
}

// Loads a request / response test case pair from our backing store
func loadTestCase(requestFile string, responseFile string) {
	requestBytes, _ := ioutil.ReadFile(requestFile) // Files are not too large...
	responseBytes, _ := ioutil.ReadFile(responseFile)
	request := string(requestBytes)

	// Retrieve HTTP request route by splitting twice and return the 2nd element
	// e.g. "GET /index.html HTTP/1.1" will yield "/index.html". That's all we need
	route := strings.SplitN(request, " ", 2)[1]
	name := strings.TrimSuffix(path.Base(requestFile), ".request")
	hash := string(hashHttpHeaders(request))

	fmt.Println("Adding %s %s (%s)\n", route, name, hash)

	testSuite[hash] = Endpoint{Route: route, Name: name, Request: requestBytes, Response: responseBytes}
}

var currentRequestFile string // TODO: Hide in closure
// Discovers request / response test cases
func discoverTestCases(path string, f os.FileInfo, err error) error {
	fmt.Printf("Visited: %s\n", path)

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

// Default handler returns Routes dictionary
func indexHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Generic HTTP API Endpoint mocking framework\n\n")
	fmt.Fprintf(w, "The following API routes are known:\n\n")
	for hash, endpoint := range testSuite {
		fmt.Println("%s %s (%s)", endpoint.Route, endpoint.Name, hash)
	}
}

func main() {
	flag.Parse()

	testSuite = make(map[string]Endpoint)

	rootDir := "endpoints" // Our test case backing store
	err := filepath.Walk(rootDir, discoverTestCases)
	fmt.Println(err)

	//commonHandlers := alice.New(loggingHandler, recoverHandler)
	//http.HandleFunc("/", commonHandlers.ThenFunc(indexHandler))
	//http.ListenAndServe(":8080", nil)

	chain := alice.New(loggingHandler, recoverHandler).ThenFunc(indexHandler)
	http.ListenAndServe(":8080", chain)
}
