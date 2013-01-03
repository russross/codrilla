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
	TimeZoneName         string
	SessionSecret        string
	CompressionThreshold int

	RedisHost     string
	RedisPassword string
	RedisDB       int64

	BrowserIDVerifyURL string
	BrowserIDAudience  string
}

const configFile = "config.json"
const scriptPath = "scripts"

var config Config
var timeZone *time.Location
var store sessions.Store

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
		http.ServeFile(w, r, "index.html")
	})

	// load Lua scripts
	db := redis.NewTCPClient(config.RedisHost, config.RedisPassword, config.RedisDB)
	defer db.Close()
	loadScripts(db, scriptPath)

	log.Fatal(http.ListenAndServe(":8080", nil))
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
	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	// get a db connection
	db := redis.NewTCPClient(config.RedisHost, config.RedisPassword, config.RedisDB)
	defer db.Close()
	if err := cron(db); err != nil {
		http.Error(w, "DB error running cron updates", http.StatusInternalServerError)
		return
	}

	// call the handler
	h(w, r, db, session)
}

type handlerAdmin func(http.ResponseWriter, *http.Request, *redis.Client, *sessions.Session)

func (h handlerAdmin) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	// get a db connection
	db := redis.NewTCPClient(config.RedisHost, config.RedisPassword, config.RedisDB)
	defer db.Close()
	if err := cron(db); err != nil {
		http.Error(w, "DB error running cron updates", http.StatusInternalServerError)
		return
	}

	// verify that the user is logged in
	if err := checkSession(db, session); err != nil {
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
	h(w, r, db, session)
}

type handlerInstructor func(http.ResponseWriter, *http.Request, *redis.Client, *sessions.Session)

func (h handlerInstructor) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	// get a db connection
	db := redis.NewTCPClient(config.RedisHost, config.RedisPassword, config.RedisDB)
	defer db.Close()
	if err := cron(db); err != nil {
		http.Error(w, "DB error running cron updates", http.StatusInternalServerError)
		return
	}

	// verify that the user is logged in
	if err := checkSession(db, session); err != nil {
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
	h(w, r, db, session)
}

type handlerStudent func(http.ResponseWriter, *http.Request, *redis.Client, *sessions.Session)

func (h handlerStudent) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	// get a db connection
	db := redis.NewTCPClient(config.RedisHost, config.RedisPassword, config.RedisDB)
	defer db.Close()
	if err := cron(db); err != nil {
		http.Error(w, "DB error running cron updates", http.StatusInternalServerError)
		return
	}

	// verify that the user is logged in
	if err := checkSession(db, session); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// call the handler
	h(w, r, db, session)
}

func writeJson(w http.ResponseWriter, r *http.Request, elt interface{}) {
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
