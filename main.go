package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/gorilla/sessions"
	"github.com/vmihailenco/redis"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Address              string
	TimeZoneName         string
	SessionSecret        string
	CompressionThreshold int

	RedisHost     string
	RedisPassword string
	RedisDB       int64

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
var pool *redis.Client

// map from filenames to sha1 hashes of all scripts that are loaded
var luaScripts = make(map[string]string)

func main() {
	// load config
	raw, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatalf("Failed to load %s: %v", configFile, err)
	}
	if err = json.Unmarshal(raw, &config); err != nil {
		log.Fatalf("Failed to decode %s: %v", configFile, err)
	}

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
		http.ServeFile(w, r, "student.html")
	})

	// load Lua scripts
	pool = redis.NewTCPClient(config.RedisHost, config.RedisPassword, config.RedisDB)
	defer pool.Close()
	loadScripts(pool, scriptPath)
	setupProblemTypes(pool)
	restoreQueue(pool)

	log.Printf("Listening on %s", config.Address)
	if err = http.ListenAndServe(config.Address, nil); err != nil {
		log.Fatal(err)
	}
}

func restoreQueue(db *redis.Client) {
	err := db.SUnionStore("queue:solution:waiting", "queue:solution:waiting", "queue:solution:processing").Err()
	if err != nil {
		log.Fatalf("DB error moving processing queue to waiting queue: %v", err)
	}

	if err = db.Del("queue:solution:processing").Err(); err != nil {
		log.Fatalf("DB error deleting processing queue: %v", err)
	}
}

func cron(db *redis.Client) error {
	now := fmt.Sprintf("%d", time.Now().In(timeZone).Unix())
	if err := db.EvalSha(luaScripts["cron"], nil, []string{now}).Err(); err != nil {
		log.Printf("Error running cron job: %v", err)
		return err
	}

	return nil
}

type handlerNoAuth func(http.ResponseWriter, *http.Request, *redis.Client, *sessions.Session)

func (h handlerNoAuth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.Path)

	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	if err := cron(pool); err != nil {
		http.Error(w, "DB error running cron updates", http.StatusInternalServerError)
		return
	}

	// call the handler
	h(w, r, pool, session)
}

type handlerAdmin func(http.ResponseWriter, *http.Request, *redis.Client, *sessions.Session)

func (h handlerAdmin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.Path)

	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	if err := cron(pool); err != nil {
		http.Error(w, "DB error running cron updates", http.StatusInternalServerError)
		return
	}

	// verify that the user is logged in
	if err := checkSession(pool, session); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// check that the user is logged in as an admin
	if session.Values["role"] != "admin" {
		log.Printf("Call to %s by non-admin", r.URL.Path)
		http.Error(w, "Must be logged in as an administrator", http.StatusForbidden)
		return
	}

	// call the handler
	h(w, r, pool, session)
}

type handlerInstructor func(http.ResponseWriter, *http.Request, *redis.Client, *sessions.Session)

func (h handlerInstructor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.Path)

	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	if err := cron(pool); err != nil {
		http.Error(w, "DB error running cron updates", http.StatusInternalServerError)
		return
	}

	// verify that the user is logged in
	if err := checkSession(pool, session); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// check that the user is logged in as an instructor or admin
	if session.Values["role"] != "admin" && session.Values["role"] != "instructor" {
		log.Printf("Call to %s by non-instructor", r.URL.Path)
		http.Error(w, "Must be logged in as an instructor", http.StatusForbidden)
		return
	}

	// call the handler
	h(w, r, pool, session)
}

type handlerInstructorJson func(http.ResponseWriter, *http.Request, *redis.Client, *sessions.Session, *json.Decoder)

func (h handlerInstructorJson) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.Path)

	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	if err := cron(pool); err != nil {
		http.Error(w, "DB error running cron updates", http.StatusInternalServerError)
		return
	}

	// verify that the user is logged in
	if err := checkSession(pool, session); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// check that the user is logged in as an instructor or admin
	if session.Values["role"] != "admin" && session.Values["role"] != "instructor" {
		log.Printf("Call to %s by non-instructor", r.URL.Path)
		http.Error(w, "Must be logged in as an instructor", http.StatusForbidden)
		return
	}

	if !checkJsonRequest(w, r) {
		return
	}

	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()

	// call the handler
	h(w, r, pool, session, decoder)
}

type handlerStudent func(http.ResponseWriter, *http.Request, *redis.Client, *sessions.Session)

func (h handlerStudent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.Path)

	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	if err := cron(pool); err != nil {
		http.Error(w, "DB error running cron updates", http.StatusInternalServerError)
		return
	}

	// verify that the user is logged in
	if err := checkSession(pool, session); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// call the handler
	h(w, r, pool, session)
}

type handlerStudentJson func(http.ResponseWriter, *http.Request, *redis.Client, *sessions.Session, *json.Decoder)

func (h handlerStudentJson) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("%s %s", r.Method, r.URL.Path)

	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	if err := cron(pool); err != nil {
		http.Error(w, "DB error running cron updates", http.StatusInternalServerError)
		return
	}

	// verify that the user is logged in
	if err := checkSession(pool, session); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	if !checkJsonRequest(w, r) {
		return
	}

	decoder := json.NewDecoder(r.Body)
	defer r.Body.Close()

	// call the handler
	h(w, r, pool, session, decoder)
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

func loadScripts(db *redis.Client, path string) {
	names, err := filepath.Glob(path + "/*.lua")
	if err != nil {
		log.Fatalf("Failed to get list of Lua scripts: %v", err)
	}

	count := 0
	for _, name := range names {
		_, key := filepath.Split(name)
		key = key[:len(key)-len(".lua")]

		var contents []byte
		if contents, err = ioutil.ReadFile(name); err != nil {
			log.Fatalf("Failed to load script %s: %v", name, err)
		}

		reply := db.ScriptLoad(string(contents))
		if err := reply.Err(); err != nil {
			log.Fatalf("Failed to load script %s into redis: %v", name, err)
		}

		luaScripts[key] = reply.Val()
		count++
	}
	log.Printf("Loaded %d Lua scripts", count)
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
