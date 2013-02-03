package main

import (
	"database/sql"
	"encoding/json"
	"github.com/gorilla/pat"
	"github.com/gorilla/sessions"
	"github.com/vmihailenco/redis"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func init() {
	r := pat.New()
	r.Add("GET", `/student/courses`, handlerStudentSQL(student_courses))
	r.Add("GET", `/student/grades/{coursetag:[\w:_\-]+$}`, handlerStudentSQL(student_grades))
	r.Add("GET", `/student/assignment/{id:\d+$}`, handlerStudentSQL(student_assignment))
	r.Add("POST", `/student/submit/{id:\d+$}`, handlerStudentJson(student_submit))
	r.Add("GET", `/student/result/{id:\d+}/{n:-1$|\d+$}`, handlerStudent(student_result))
	http.Handle("/student/", r)
}

type CourseListing struct {
	Tag             string
	Name            string
	Close           time.Time
	Instructors     []string
	OpenAssignments []*AssignmentListing
	NextAssignment  *AssignmentListing
}

func getCourseListing(db *sql.DB, course *CourseDB, student *StudentDB) *CourseListing {
	now := time.Now()
	elt := &CourseListing{
		Tag:             course.Tag,
		Name:            course.Name,
		Close:           course.Close,
		Instructors:     []string{},
		OpenAssignments: []*AssignmentListing{},
	}
	for email, _ := range course.Instructors {
		elt.Instructors = append(elt.Instructors, email)
	}
	var next *AssignmentDB
	for _, asst := range course.Assignments {
		if now.After(asst.Open) && now.Before(asst.Close) {
			elt.OpenAssignments = append(elt.OpenAssignments, getAssignmentListing(db, asst, student))
		} else if now.Before(asst.Open) && (next == nil || asst.Open.Before(next.Open)) {
			next = asst
		}
	}
	sort.Sort(AssignmentsByClose(elt.OpenAssignments))
	if next != nil {
		elt.NextAssignment = getAssignmentListing(db, next, student)
	}
	return elt
}

type AssignmentListing struct {
	ID         int64
	Name       string
	Open       time.Time
	Close      time.Time
	Active     bool
	ForCredit  bool
	Attempts   int
	ToBeGraded int
	Passed     bool
}

type AssignmentsByClose []*AssignmentListing

func (p AssignmentsByClose) Len() int           { return len(p) }
func (p AssignmentsByClose) Less(i, j int) bool { return p[i].Close.Before(p[j].Close) }
func (p AssignmentsByClose) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func getAssignmentListing(db *sql.DB, asst *AssignmentDB, student *StudentDB) *AssignmentListing {
	now := time.Now()
	elt := &AssignmentListing{
		ID:        asst.ID,
		Name:      asst.Problem.Name,
		Open:      asst.Open,
		Close:     asst.Close,
		Active:    now.After(asst.Open) && now.Before(asst.Close),
		ForCredit: asst.ForCredit,
	}
	if student != nil {
		sol, present := student.SolutionsByAssignment[asst.ID]
		if present {
			elt.Attempts = len(sol.SubmissionsInOrder)
			for i := len(sol.SubmissionsInOrder) - 1; i >= 0; i-- {
				if len(sol.SubmissionsInOrder[i].GradeReport) > 0 {
					// record whether the last graded submission was a pass
					elt.Passed = sol.SubmissionsInOrder[i].Passed
					break
				}
				elt.ToBeGraded++
			}
		}
	}
	return elt
}

// get a list of current courses and assignments for this student
type StudentCoursesResponse struct {
	Email   string
	Name    string
	Courses []*CourseListing
}

func student_courses(w http.ResponseWriter, r *http.Request, db *sql.DB, student *StudentDB, session *sessions.Session) {
	resp := &StudentCoursesResponse{
		Email:   student.Email,
		Name:    student.Name,
		Courses: []*CourseListing{},
	}
	var tags []string
	for tag, _ := range student.Courses {
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	for _, tag := range tags {
		resp.Courses = append(resp.Courses, getCourseListing(db, student.Courses[tag], student))
	}

	writeJson(w, r, resp)
}

// get a list of assignments for this student with grade info
func student_grades(w http.ResponseWriter, r *http.Request, db *sql.DB, student *StudentDB, session *sessions.Session) {
	courseTag := r.URL.Query().Get(":coursetag")

	course, present := student.Courses[courseTag]
	if !present {
		log.Printf("student not enrolled in course/course does not exist: %s", courseTag)
		http.Error(w, "course not found", http.StatusNotFound)
		return
	}

	list := []*AssignmentListing{}
	now := time.Now()
	for _, asst := range course.Assignments {
		if !now.Before(asst.Open) {
			list = append(list, getAssignmentListing(db, asst, student))
		}
	}
	sort.Sort(AssignmentsByClose(list))

	writeJson(w, r, list)
}

type StudentAssignmentResponse struct {
	CourseTag   string
	CourseName  string
	ProblemType *ProblemType
	ProblemData map[string]interface{}
	Assignment  *AssignmentListing
	Attempt     map[string]interface{}
}

func student_assignment(w http.ResponseWriter, r *http.Request, db *sql.DB, student *StudentDB, session *sessions.Session) {
	id, err := strconv.ParseInt(r.URL.Query().Get(":id"), 10, 64)
	if err != nil {
		log.Printf("Bad ID: %s", r.URL.Query().Get(":id"))
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// find the assignment
	asst, present := assignmentsByID[id]
	if !present {
		log.Printf("No such assignment: %d", id)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// make sure the assignment is active
	now := time.Now()
	if now.Before(asst.Open) || now.After(asst.Close) {
		log.Printf("Assignment is not active: %d", asst.ID)
		http.Error(w, "Assignment not active", http.StatusForbidden)
		return
	}

	// find the course
	course := asst.Course

	// make sure the course is active
	if now.After(course.Close) {
		log.Printf("Course is not active: %s", course.Tag)
		http.Error(w, "Course not active", http.StatusForbidden)
		return
	}

	// make sure the student is in the course
	if _, present := student.Courses[course.Tag]; !present {
		log.Printf("Student not enrolled in course: %s", course.Tag)
		http.Error(w, "Not enrolled in course", http.StatusForbidden)
		return
	}

	// get the problem
	problem := asst.Problem

	// make sure we still know about this problem type
	problemType, present := problemTypes[problem.Type]
	if !present {
		log.Printf("Problem %d has unknown type %s", problem.ID, problem.Type)
		http.Error(w, "Unknown problem type", http.StatusInternalServerError)
		return
	}

	// filter problem fields down to what the student is allowed to see
	data := make(map[string]interface{})
	for _, field := range problemType.FieldList {
		if value, present := problem.Data[field.Name]; present && field.Student == "view" {
			data[field.Name] = value
		}
	}

	// get the most recent student attempt (if any)
	sol, present := student.SolutionsByAssignment[asst.ID]
	attempt := make(map[string]interface{})
	if present && len(sol.SubmissionsInOrder) > 0 {
		attempt = sol.SubmissionsInOrder[len(sol.SubmissionsInOrder)-1].Submission
	}

	resp := &StudentAssignmentResponse{
		CourseTag:   course.Tag,
		CourseName:  course.Name,
		ProblemType: problemType,
		ProblemData: data,
		Assignment:  getAssignmentListing(db, asst, student),
		Attempt:     attempt,
	}

	writeJson(w, r, resp)
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
	notifyGrader <- true
}
