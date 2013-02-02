package main

import (
	"encoding/json"
	"github.com/gorilla/pat"
	"github.com/gorilla/sessions"
	"github.com/vmihailenco/redis"
	"log"
	"net/http"
	"strings"
)

func init() {
	r := pat.New()
	r.Add("GET", `/student/courses`, handlerStudent(student_courses))
	r.Add("GET", `/student/grades/{coursetag:[\w:_\-]+$}`, handlerStudent(student_grades))
	r.Add("GET", `/student/assignment/{id:\d+$}`, handlerStudent(student_assignment))
	r.Add("POST", `/student/submit/{id:\d+$}`, handlerStudentJson(student_submit))
	r.Add("GET", `/student/result/{id:\d+}/{n:-1$|\d+$}`, handlerStudent(student_result))
	http.Handle("/student/", r)
}

// get a list of current courses and assignments for this student
func student_courses(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	email := session.Values["email"].(string)

	iface := db.EvalSha(luaScripts["studentlistcourses"], nil, []string{email})
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

// get a list of assignment grades for this student
func student_grades(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	email := session.Values["email"].(string)
	courseTag := r.URL.Query().Get(":coursetag")

	iface := db.EvalSha(luaScripts["studentlistgrades"], nil, []string{email, courseTag})
	if iface.Err() != nil {
		log.Printf("DB error getting student %s course grades for %s: %v", email, courseTag, iface.Err())
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

type StudentAssignment struct {
	CourseName  string
	CourseTag   string
	ProblemType *ProblemType
	ProblemData map[string]interface{}
	Assignment  map[string]interface{}
	Attempt     map[string]interface{}
	Passed      bool
}

func student_assignment(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	email := session.Values["email"].(string)
	id := r.URL.Query().Get(":id")

	iface := db.EvalSha(luaScripts["studentassignment"], nil, []string{email, id})
	if iface.Err() != nil {
		log.Printf("DB error getting assignment %s for student %s: %v", id, email, iface.Err())
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	response := new(StudentAssignment)
	if err := json.Unmarshal([]byte(iface.Val().(string)), response); err != nil {
		log.Printf("Unable to parse JSON response from DB: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// filter problem fields down to what the student is allowed to see
	data := make(map[string]interface{})
	for _, field := range response.ProblemType.FieldList {
		if value, present := response.ProblemData[field.Name]; present && field.Student == "view" {
			data[field.Name] = value
		}
	}
	response.ProblemData = data

	writeJson(w, r, response)
}

type StudentResult struct {
	CourseName  string
	CourseTag   string
	ProblemType *ProblemType
	ProblemData map[string]interface{}
	Assignment  map[string]interface{}
	Attempt     map[string]interface{}
	ResultData  map[string]interface{}
}

func student_result(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	email := session.Values["email"].(string)
	id := r.URL.Query().Get(":id")
	n := r.URL.Query().Get(":n")

	iface := db.EvalSha(luaScripts["studentresult"], nil, []string{email, id, n})
	if iface.Err() != nil {
		if strings.Contains(iface.Err().Error(), "404 Not Found") {
			log.Printf("Assignment result not found")
			http.Error(w, "Not found", http.StatusNotFound)
		} else {
			log.Printf("DB error getting result %s for student %s: %v", id, email, iface.Err())
			http.Error(w, "DB error", http.StatusInternalServerError)
		}
		return
	}

	response := new(StudentResult)
	if err := json.Unmarshal([]byte(iface.Val().(string)), response); err != nil {
		log.Printf("Unable to parse JSON response from DB: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// filter problem fields down to what the student is allowed to see
	data := make(map[string]interface{})
	for _, field := range response.ProblemType.FieldList {
		if value, present := response.ProblemData[field.Name]; present && field.Student == "view" {
			data[field.Name] = value
		}
	}
	response.ProblemData = data

	writeJson(w, r, response)
}

func student_submit(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session, decoder *json.Decoder) {
	email := session.Values["email"].(string)
	assignmentID := r.URL.Query().Get(":id")

	submission := make(map[string]interface{})
	if err := decoder.Decode(&submission); err != nil {
		log.Printf("Failure decoding JSON request: %v", err)
		http.Error(w, "Failure decoding JSON request", http.StatusBadRequest)
		return
	}

	// make sure this is an active assignment for a course the student is in
	// and get the problem type description object
	iface := db.EvalSha(luaScripts["getproblemtypeforsubmission"], []string{}, []string{email, assignmentID})
	if iface.Err() != nil {
		log.Printf("DB error checking if submission is permitted: %v", iface.Err())
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	problemTypeJson := iface.Val().(string)

	// decode the problem object
	problemType := new(ProblemType)
	if err := json.Unmarshal([]byte(problemTypeJson), problemType); err != nil {
		log.Printf("Unable to parse JSON problem type response from DB: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// filter it down to expected student fields
	filtered := make(map[string]interface{})

	for _, field := range problemType.FieldList {
		if value, present := submission[field.Name]; present && field.Student == "edit" {
			filtered[field.Name] = value
		} else if field.Student == "edit" {
			log.Printf("Missing %s field in submission", field.Name)
			http.Error(w, "Submission missing required field", http.StatusBadRequest)
			return
		}
	}

	attemptJson, err := json.Marshal(filtered)
	if err != nil {
		log.Printf("Error encoding submission as JSON: %v", err)
		http.Error(w, "Error encoding submission as JSON", http.StatusInternalServerError)
		return
	}

	// now save the submission and trigger the grader
	iface = db.EvalSha(luaScripts["studentsubmit"], []string{}, []string{email, assignmentID, string(attemptJson)})
	if iface.Err() != nil {
		log.Printf("DB error saving submission: %v", iface.Err())
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
}
