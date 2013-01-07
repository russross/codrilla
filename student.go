package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/pat"
	"github.com/gorilla/sessions"
	"github.com/vmihailenco/redis"
	"log"
	"net/http"
	"time"
)

func init() {
	r := pat.New()
	r.Add("GET", `/student/list/courses`, handlerStudent(student_list_courses))
	http.Handle("/student/", r)
}

// get a list of current courses and assignments for this student
func student_list_courses(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	now := fmt.Sprintf("%d", time.Now().In(timeZone).Unix())
	email := session.Values["email"].(string)
	email = "smoore6@dmail.dixie.edu"

	iface := db.EvalSha(luaScripts["studentlistcourses"], nil, []string{email, now})
	if iface.Err() != nil {
		log.Printf("DB error getting student course listing for %s: %v", email, iface.Err())
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	var response interface{}
	if err := json.Unmarshal([]byte(iface.Val().(string)), &response); err != nil {
		log.Printf("Unable to parse JSON response from DB: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	writeJson(w, r, response)
}
