package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"github.com/gorilla/pat"
	"github.com/gorilla/sessions"
	"github.com/vmihailenco/redis"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
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

	// load time zone
	if timeZone, err = time.LoadLocation(config.TimeZoneName); err != nil {
		log.Fatalf("Failed to load timezone %s: %v", config.TimeZoneName, err)
	}

	// set up session store
	store = sessions.NewCookieStore([]byte(config.SessionSecret))

	// set up web server
	r := pat.New()
	r.Add("POST", `/auth/login/browserid`, handlerNoAuth(auth_login_browserid))

	http.Handle("/auth/", r)
	http.Handle("/css/", http.StripPrefix("/css", http.FileServer(http.Dir("css"))))
	http.Handle("/js/", http.StripPrefix("/js", http.FileServer(http.Dir("js"))))
	http.Handle("/img/", http.StripPrefix("/img", http.FileServer(http.Dir("img"))))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}

type handlerNoAuth func(http.ResponseWriter, *http.Request, *redis.Client, *sessions.Session)

func (h handlerNoAuth) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// get a db connection
	db := redis.NewTCPClient(config.RedisHost, config.RedisPassword, config.RedisDB)
	defer db.Close()

	// get the session (or create a new one)
	session, _ := store.Get(r, "codrilla-session")

	// call the handler
	h(w, r, db, session)
}

func auth_login_browserid(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	// get the assertion from the submitted form data
	assertion := strings.TrimSpace(r.FormValue("Assertion"))
	if assertion == "" {
		http.Error(w, "Missing BrowserID Assertion", http.StatusBadRequest)
		return
	}

	// check for a successful login
	email, err := browserid_verify(assertion)
	if err != nil {
		http.Error(w, "Error while verifying BrowserID login: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if email == "" {
		http.Error(w, "Login failed", http.StatusForbidden)
		return
	}

	log.Printf("BrowserID login for [%s]", email)

	// create a login session cookie
	createLoginSession(w, r, db, session, email)
}

func createLoginSession(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session, email string) {
	// start by assuming this is a student
	role := "student"

	// is this an instructor?
	b := db.SIsMember("index:instructors:all", email)
	if err := b.Err(); err != nil {
		log.Printf("DB error checking if user %s is an instructor: %v", email, err)
		http.Error(w, "Database error checking if user is an instructor", http.StatusInternalServerError)
		return
	}
	if b.Val() {
		role = "instructor"
	}

	// is this an admin?
	b = db.SIsMember("index:administrators", email)
	if err := b.Err(); err != nil {
		log.Printf("DB error checking if user %s is an administrator: %v", email, err)
		http.Error(w, "Database error checking if user is an administrator", http.StatusInternalServerError)
		return
	}
	if b.Val() {
		role = "admin"
	}

	session.Values["role"] = role
	session.Values["email"] = email

	// compute an expiration time
	// we'll set it to 30 days, but set it to 4:00am
	// so it is unlikely to expire while someone is working
	now := time.Now().In(timeZone)
	expires := time.Date(now.Year(), now.Month(), now.Day()+30, 4, 0, 0, 0, timeZone)
	session.Values["expires"] = expires.Unix()
	session.Save(r, w)

	//http.Redirect(w, r, "/", http.StatusFound)
	writeJson(w, r, map[string]string{"Email": email})
}

type BrowserIDVerificationResponse struct {
	Status   string
	Email    string
	Audience string
	Expires  int64
	Issuer   string
	Reason   string
}

func browserid_verify(assertion string) (email string, err error) {
	// try to verify the assertion with the Persona server
	resp, err := http.PostForm(
		config.BrowserIDVerifyURL,
		url.Values{
			"assertion": {assertion},
			"audience":  {config.BrowserIDAudience},
		})

	if err != nil {
		log.Printf("Failure contacting BrowserID verification server: %v", err)
		return "", fmt.Errorf("Failure contacting verification server: %v", err)
	}
	defer resp.Body.Close()

	// decode the body
	verify := new(BrowserIDVerificationResponse)
	if err = json.NewDecoder(resp.Body).Decode(verify); err != nil {
		log.Printf("Failure decoding BrowserID verification: %v", err)
		return "", fmt.Errorf("Failure decoding verification: %v", err)
	}
	if verify.Status == "failure" {
		log.Printf("Failed BrowserID login: %s", verify.Reason)
		return "", nil
	} else if verify.Status != "okay" {
		log.Printf("Failed BrowserID login with unknown status: %s", verify.Status)
		return "", fmt.Errorf("Failed with unknown verification status: %s", verify.Status)
	}

	// sanity checks
	if !strings.Contains(verify.Email, "@") {
		return "", fmt.Errorf("Invalid email address from BrowserID login: [%s]", verify.Email)
	}
	if verify.Audience != config.BrowserIDAudience {
		return "", fmt.Errorf("Wrong BrowserID audience: [%s] instead of [%s]", verify.Audience, config.BrowserIDAudience)
	}

	return strings.ToLower(verify.Email), nil
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
