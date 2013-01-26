package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/pat"
	"github.com/gorilla/sessions"
	"github.com/vmihailenco/redis"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func init() {
	r := pat.New()
	r.Add("POST", `/auth/login/browserid`, handlerNoAuth(auth_login_browserid))
	r.Add("POST", `/auth/logout`, handlerNoAuth(auth_logout))
	http.Handle("/auth/", r)
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

func auth_logout(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	expires := time.Date(1970, 0, 0, 0, 0, 1, 0, time.UTC)

	// clear the session cookie
	session.Options = &sessions.Options{
		Path:   "/",
		MaxAge: -1,
	}
	sessions.Save(r, w)

	// clear the other cookies
	http.SetCookie(w, &http.Cookie{
		Name:    "codrilla-email",
		Path:    "/",
		Expires: expires,
		MaxAge:  -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name:    "codrilla-role",
		Path:    "/",
		Expires: expires,
		MaxAge:  -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name:    "codrilla-expires",
		Path:    "/",
		Expires: expires,
		MaxAge:  -1,
	})
	log.Printf("Logout")
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

	http.SetCookie(w, &http.Cookie{
		Name:    "codrilla-email",
		Value:   email,
		Path:    "/",
		Expires: expires,
		MaxAge:  int(expires.Sub(now).Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name:    "codrilla-role",
		Value:   role,
		Path:    "/",
		Expires: expires,
		MaxAge:  int(expires.Sub(now).Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name:    "codrilla-expires",
		Value:   fmt.Sprintf("%d", expires.Unix()),
		Path:    "/",
		Expires: expires,
		MaxAge:  int(expires.Sub(now).Seconds()),
	})

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

func checkSession(db *redis.Client, session *sessions.Session) error {
	//session.Values["email"] = "russ@dixie.edu"
	//session.Values["role"] = "instructor"
	session.Values["email"] = "jmacdon1@dmail.dixie.edu"
	session.Values["role"] = "student"
	session.Values["expires"] = time.Now().Add(time.Hour).Unix()

	// make sure someone is logged in
	if _, present := session.Values["email"]; !present {
		log.Printf("Must be logged in")
		return fmt.Errorf("Must be logged in")
	}
	email := session.Values["email"].(string)
	if email == "" {
		log.Printf("Must be logged in")
		return fmt.Errorf("Must be logged in")
	}

	// check if the session is expired
	now := time.Now().In(timeZone)
	expires := time.Unix(session.Values["expires"].(int64), 0)
	if expires.Before(now) {
		log.Printf("Expired session")
		return fmt.Errorf("Session expired")
	}

	// validate the role
	role := session.Values["role"].(string)
	switch role {
	case "admin":
		// verify that this email is still on the admin list
		reply := db.SIsMember("index:administrators", email)
		if err := reply.Err(); err != nil {
			log.Printf("DB error checking admin list for %s: %v", email, err)
			return fmt.Errorf("DB error checking admin list")
		}
		if !reply.Val() {
			log.Printf("Session says admin, but user %s is not on the admin list", email)
			return fmt.Errorf("Must be logged in as an administrator")
		}

	case "instructor":
		// verify that this email is still on the instructors list
		reply := db.SIsMember("index:instructors:all", email)
		if err := reply.Err(); err != nil {
			log.Printf("DB error checking instructors list for %s: %v", email, err)
			return fmt.Errorf("DB error checking instructors list")
		}
		if !reply.Val() {
			log.Printf("Session says instructor, but user %s is not on the instructor list", email)
			return fmt.Errorf("Must be logged in as an administrator")
		}

	case "student":
		// verify that this email is still on the active student list
		reply := db.SIsMember("index:students:active", email)
		if err := reply.Err(); err != nil {
			log.Printf("DB error checking active student list for %s: %v", email, err)
			return fmt.Errorf("DB error checking active student list")
		}
		if !reply.Val() {
			log.Printf("Session says student, but user %s is not on the active student list", email)
			return fmt.Errorf("Must be logged in as an active student")
		}

	default:
		log.Printf("Unrecognized role in session: %s", role)
		return fmt.Errorf("Invalid role in session")
	}

	return nil
}
