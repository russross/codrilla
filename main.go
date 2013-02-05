package main

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"github.com/gorilla/sessions"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Address              string
	TimeZoneName         string
	SessionSecret        string
	CompressionThreshold int

	DatabaseName  string
	LogFileName   string
	GraderAddress string

	BrowserIDVerifyURL string
	BrowserIDAudience  string

	GoogleVerifyURL    string
	GoogleGetEmailURL  string
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURI  string

	StudentEmailDomain string
}

const configFile = "config.json"
const scriptPath = "scripts"

var config Config
var timeZone *time.Location
var store sessions.Store

func main() {
	// load config
	raw, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatalf("Failed to load %s: %v", configFile, err)
	}
	if err = json.Unmarshal(raw, &config); err != nil {
		log.Fatalf("Failed to decode %s: %v", configFile, err)
	}

	// set up logger
	logfile, err := os.OpenFile(config.LogFileName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
	if err != nil {
		log.Fatalf("Failed to open logfile %s: %v", config.LogFileName, err)
	}
	defer logfile.Close()
	log.SetOutput(logfile)

	// load time zone
	if timeZone, err = time.LoadLocation(config.TimeZoneName); err != nil {
		log.Fatalf("Failed to load timezone %s: %v", config.TimeZoneName, err)
	}

	// set up session store
	store = sessions.NewCookieStore([]byte(config.SessionSecret))

	// set up web server
	http.Handle("/css/", http.StripPrefix("/css", http.FileServer(http.Dir("css"))))
	http.Handle("/js/", http.StripPrefix("/js", http.FileServer(http.Dir("js"))))
	http.Handle("/img/", http.StripPrefix("/img", http.FileServer(http.Dir("img"))))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, "index.html")
	})

	// load problem types
	setupProblemTypes()

	// connect to database
	initDatabase()

	// start grader
	notifyGrader = make(chan int64, 100)
	go gradeDaemon()

	log.Printf("Listening on %s", config.Address)
	if err = http.ListenAndServe(config.Address, nil); err != nil {
		log.Fatal(err)
	}
}

type handlerNoAuth func(http.ResponseWriter, *http.Request, *sessions.Session)

func (h handlerNoAuth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.Path)

	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	// get a read lock
	mutex.RLock()
	defer mutex.RUnlock()

	// call the handler
	h(w, r, session)
}

type handlerInstructor func(http.ResponseWriter, *http.Request, *InstructorDB)

func (h handlerInstructor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.Path)

	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	// get a read lock
	mutex.RLock()
	defer mutex.RUnlock()

	// verify that the user is logged in
	email, err := checkSession(session)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	instructor, present := instructorsByEmail[email]
	if !present {
		log.Printf("InstructorDB not found: %s", email)
		http.Error(w, "Instructor record not found", http.StatusNotFound)
		return
	}

	// check that the user is logged in as an instructor or admin
	if session.Values["role"] != "admin" && session.Values["role"] != "instructor" {
		log.Printf("Call to %s by non-instructor", r.URL.Path)
		http.Error(w, "Must be logged in as an instructor", http.StatusForbidden)
		return
	}

	// call the handler
	h(w, r, instructor)
}

type handlerInstructorJson func(http.ResponseWriter, *http.Request, *sql.DB, *InstructorDB, *json.Decoder)

func (h handlerInstructorJson) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.Path)

	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	// get a read/write lock
	mutex.Lock()
	defer mutex.Unlock()

	// verify that the user is logged in
	email, err := checkSession(session)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if !checkJsonRequest(w, r) {
		return
	}

	instructor, present := instructorsByEmail[email]
	if !present {
		log.Printf("InstructorDB not found: %s", email)
		http.Error(w, "Instructor record not found", http.StatusNotFound)
		return
	}

	// check that the user is logged in as an instructor or admin
	if session.Values["role"] != "admin" && session.Values["role"] != "instructor" {
		log.Printf("Call to %s by non-instructor", r.URL.Path)
		http.Error(w, "Must be logged in as an instructor", http.StatusForbidden)
		return
	}

	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()

	// call the handler
	h(w, r, database, instructor, decoder)
}

type handlerStudent func(http.ResponseWriter, *http.Request, *StudentDB)

func (h handlerStudent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.Path)

	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	// get a read lock
	mutex.RLock()
	defer mutex.RUnlock()

	// verify that the user is logged in
	email, err := checkSession(session)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	student, present := studentsByEmail[email]
	if !present {
		log.Printf("StudentDB not found: %s", email)
		http.Error(w, "Student record not found", http.StatusNotFound)
		return
	}

	h(w, r, student)
}

type handlerStudentJson func(http.ResponseWriter, *http.Request, *sql.DB, *StudentDB, *json.Decoder)

func (h handlerStudentJson) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.Path)

	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	// get a read/write lock
	mutex.Lock()
	defer mutex.Unlock()

	// verify that the user is logged in
	email, err := checkSession(session)
	if err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if !checkJsonRequest(w, r) {
		return
	}

	student, present := studentsByEmail[email]
	if !present {
		log.Printf("StudentDB not found: %s", email)
		http.Error(w, "Student record not found", http.StatusNotFound)
		return
	}

	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()

	// call the handler
	h(w, r, database, student, decoder)
}

func writeJson(w http.ResponseWriter, r *http.Request, elt interface{}) {
	if !strings.Contains(r.Header.Get("Accept"), "application/json") &&
		!strings.Contains(r.Header.Get("Accept"), "*/*") {
		log.Printf("Accept header missing JSON: Accept is %s", r.Header.Get("Accept"))
		http.Error(w, "Client does not accept JSON response; must include Accept: application/json in request", http.StatusBadRequest)
		return
	}
	raw, err := json.MarshalIndent(elt, "", "    ")
	if err != nil {
		log.Printf("Error encoding result as JSON: %v", err)
		http.Error(w, "Failure encoding result as JSON", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	size, actual := 0, 0
	if len(raw) >= config.CompressionThreshold && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		gz.Write(raw)
		gz.Close()
		w.Header().Set("Content-Length", strconv.Itoa(buf.Len()))
		size = len(buf.Bytes())
		actual, err = w.Write(buf.Bytes())
	} else {
		w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
		size = len(raw)
		actual, err = w.Write(raw)
	}
	if err != nil {
		log.Printf("Error writing result: %v", err)
		http.Error(w, "Failure writing JSON result", http.StatusInternalServerError)
	} else if size != actual {
		log.Printf("Output truncated")
		http.Error(w, "Output truncated", http.StatusInternalServerError)
	}
}

func checkJsonRequest(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != "POST" {
		log.Printf("JSON request called with method %s", r.Method)
		http.Error(w, "Not found", http.StatusNotFound)
		return false
	}

	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		log.Printf("JSON request called with Content-Type %s", r.Header.Get("Content-Type"))
		http.Error(w, "Request must be in JSON format; must include Content-Type: application/json in request", http.StatusBadRequest)
		return false
	}

	return true
}
