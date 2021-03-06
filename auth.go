package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/pat"
	"github.com/gorilla/sessions"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func init() {
	r := pat.New()
	r.Add("POST", `/auth/login/browserid`, handlerNoAuth(auth_login_browserid))
	r.Add("GET", `/auth/login/google`, handlerNoAuth(auth_login_google))
	r.Add("POST", `/auth/logout`, handlerNoAuth(auth_logout))
	r.Add("GET", `/auth/time`, http.HandlerFunc(auth_time))
	http.Handle("/auth/", r)
}

func auth_time(w http.ResponseWriter, r *http.Request) {
	writeJson(w, r, time.Now().In(timeZone))
}

func auth_login_browserid(w http.ResponseWriter, r *http.Request, session *sessions.Session) {
	// get the assertion from the submitted form data
	assertion := strings.TrimSpace(r.FormValue("Assertion"))
	if assertion == "" {
		log.Printf("Missing BrowserID assertion")
		http.Error(w, "Missing BrowserID assertion", http.StatusBadRequest)
		return
	}

	// check for a successful login
	email, err := browserid_verify(assertion)
	if err != nil {
		log.Printf("Error while verifying BrowserID login: %v", err)
		http.Error(w, "Error while verifying BrowserID login", http.StatusInternalServerError)
		return
	}
	if email == "" {
		log.Printf("BrowserID login failed")
		http.Error(w, "Login failed", http.StatusForbidden)
		return
	}

	log.Printf("BrowserID login for [%s]", email)

	// create a login session cookie
	createLoginSession(w, r, session, email, false)
}

func auth_login_google(w http.ResponseWriter, r *http.Request, session *sessions.Session) {
	errorcode := strings.TrimSpace(r.URL.Query().Get("error"))
	if errorcode != "" {
		log.Printf("Error from Google OAuth2.0 login attempt: %s", errorcode)
		http.Error(w, "Error from Google OAuth2.0 login attempt", http.StatusForbidden)
		return
	}

	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		log.Printf("Missing Google OAuth2.0 code")
		http.Error(w, "Missing Google OAuth2.0 code", http.StatusBadRequest)
		return
	}

	// check for a successful login
	email, err := google_verify(code)
	if err != nil {
		log.Printf("Error while verifying Google OAuth2.0 code: %v", err)
		http.Error(w, "Error while verifying Google OAuth2.0 code", http.StatusInternalServerError)
		return
	}
	if email == "" {
		log.Printf("Google OAuth2.0 login failed")
		http.Error(w, "Login failed", http.StatusForbidden)
		return
	}

	log.Printf("Google OAuth2.0 login for [%s]", email)

	// create a login session cookie
	createLoginSession(w, r, session, email, true)
}

func auth_logout(w http.ResponseWriter, r *http.Request, session *sessions.Session) {
	expires := time.Date(1970, 0, 0, 0, 0, 1, 0, time.UTC)

	// clear the session cookie
	session.Options = &sessions.Options{
		Path:   "/",
		MaxAge: -1,
	}
	sessions.Save(r, w)

	// clear the other cookies
	email, err := r.Cookie("codrilla-email")
	if err != nil {
		email = nil
	}
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
	if email == nil {
		log.Printf("Logout")
	} else {
		log.Printf("Logout [%s]", email.Value)
	}
}

func createLoginSession(w http.ResponseWriter, r *http.Request, session *sessions.Session, email string, redirect bool) {
	// start by assuming this is a student
	role := "student"

	// is this an instructor?
	if _, present := instructorsByEmail[email]; present {
		role = "instructor"
	}

	// is this an admin?
	if _, present := administratorsByEmail[email]; present {
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

	if redirect {
		http.Redirect(w, r, "/", http.StatusFound)
	} else {
		writeJson(w, r, map[string]string{"Email": email})
	}
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

type GoogleOAuth20VerificationResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

type GoogleUserinfoResponse struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
}

func google_verify(code string) (email string, err error) {
	// try to verify the code with Google server
	resp, err := http.PostForm(
		config.GoogleVerifyURL,
		url.Values{
			"code":          {code},
			"client_id":     {config.GoogleClientID},
			"client_secret": {config.GoogleClientSecret},
			"redirect_uri":  {config.GoogleRedirectURI},
			"grant_type":    {"authorization_code"},
		})

	if err != nil {
		log.Printf("Failure contacting Google OAuth2.0 verification server: %v", err)
		return "", fmt.Errorf("Failure contacting verification server: %v", err)
	}
	if resp.StatusCode != 200 {
		log.Printf("Google OAuth2.0 returned a non-200 response code: %d", resp.StatusCode)
		return "", fmt.Errorf("Google server returned an error code")
	}
	defer resp.Body.Close()

	// decode the body
	verify := new(GoogleOAuth20VerificationResponse)
	if err = json.NewDecoder(resp.Body).Decode(verify); err != nil {
		log.Printf("Failure decoding Google OAuth2.0 verification: %v", err)
		return "", fmt.Errorf("Failure decoding verification")
	}

	// sanity checks
	if verify.AccessToken == "" {
		return "", fmt.Errorf("Empty access token")
	}
	if verify.ExpiresIn < 1 {
		return "", fmt.Errorf("Access token already expired")
	}
	if verify.TokenType != "Bearer" {
		log.Printf("Token type was [%s]", verify.TokenType)
		return "", fmt.Errorf("Non-bearer token type returned")
	}

	// use the access token to get the email address
	client := &http.Client{}
	req, err := http.NewRequest("GET", config.GoogleGetEmailURL, nil)
	if err != nil {
		log.Printf("Error creating request object to get email address: %v", err)
		return "", fmt.Errorf("Error creating request to get email address")
	}
	req.Header.Set("Authorization", "OAuth "+verify.AccessToken)
	if resp, err = client.Do(req); err != nil {
		log.Printf("Request to get email address failed: %v", err)
		return "", fmt.Errorf("Request to get email address failed")
	}
	if resp.StatusCode != 200 {
		log.Printf("Google userinfo returned a non-200 response code: %d", resp.StatusCode)
		return "", fmt.Errorf("Google userinfo server returned an error code")
	}
	defer resp.Body.Close()

	// decode the body
	info := new(GoogleUserinfoResponse)
	if err = json.NewDecoder(resp.Body).Decode(info); err != nil {
		log.Printf("Failure decoding Google userinfo: %v", err)
		return "", fmt.Errorf("Failure decoding userinfo")
	}

	if info.Email == "" {
		return "", fmt.Errorf("Empty email address")
	}
	if !info.VerifiedEmail {
		log.Printf("Warning: unverified email address for [%s]", info.Email)
	}

	return strings.ToLower(info.Email), nil
}

func checkSession(session *sessions.Session) (email string, err error) {
	// make sure someone is logged in
	if _, present := session.Values["email"]; !present {
		log.Printf("Must be logged in")
		return "", fmt.Errorf("Must be logged in")
	}
	email = session.Values["email"].(string)
	if email == "" {
		log.Printf("Must be logged in")
		return "", fmt.Errorf("Must be logged in")
	}

	// check if the session is expired
	now := time.Now().In(timeZone)
	expires := time.Unix(session.Values["expires"].(int64), 0)
	if expires.Before(now) {
		log.Printf("Expired session")
		return "", fmt.Errorf("Session expired")
	}

	// validate the role
	role := session.Values["role"].(string)
	switch role {
	case "admin":
		// verify that this email is still on the admin list
		if _, present := administratorsByEmail[email]; !present {
			log.Printf("Session says admin, but user %s is not on the admin list", email)
			return "", fmt.Errorf("Must be logged in as an administrator")
		}

	case "instructor":
		// verify that this email is still on the instructors list
		if _, present := instructorsByEmail[email]; !present {
			log.Printf("Session says instructor, but user %s is not on the instructor list", email)
			return "", fmt.Errorf("Must be logged in as an instructor")
		}

	case "student":
		// verify that this email is still on the active student list
		if _, present := studentsByEmail[email]; !present {
			log.Printf("Session says student, but user %s is not on the student list", email)
			return "", fmt.Errorf("Student that is logged in is not active in any courses")
		}

	default:
		log.Printf("Unrecognized role in session: %s", role)
		return "", fmt.Errorf("Invalid role in session")
	}

	remaining := expires.Sub(now)
	remaining -= remaining % 1000000000
	log.Printf("  %s: %s expires in %v", role, email, remaining)

	return
}
