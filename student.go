package main

import (
	"database/sql"
	"encoding/json"
	"github.com/gorilla/pat"
	"log"
	"net/http"
	"sort"
	"strconv"
	"time"
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

type CourseListing struct {
	Tag             string
	Name            string
	Close           time.Time
	Instructors     []string
	OpenAssignments []*AssignmentListing
	NextAssignment  *AssignmentListing
}

func getCourseListing(course *CourseDB, student *StudentDB) *CourseListing {
	now := time.Now().In(timeZone)
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
			elt.OpenAssignments = append(elt.OpenAssignments, getAssignmentListing(asst, student))
		} else if now.Before(asst.Open) && (next == nil || asst.Open.Before(next.Open)) {
			next = asst
		}
	}
	sort.Sort(AssignmentsByClose(elt.OpenAssignments))
	if next != nil {
		elt.NextAssignment = getAssignmentListing(next, student)
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

type AssignmentsByOpen []*AssignmentListing

func (p AssignmentsByOpen) Len() int           { return len(p) }
func (p AssignmentsByOpen) Less(i, j int) bool { return p[i].Open.Before(p[j].Open) }
func (p AssignmentsByOpen) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

type AssignmentsByClose []*AssignmentListing

func (p AssignmentsByClose) Len() int           { return len(p) }
func (p AssignmentsByClose) Less(i, j int) bool { return p[i].Close.Before(p[j].Close) }
func (p AssignmentsByClose) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

func getAssignmentListing(asst *AssignmentDB, student *StudentDB) *AssignmentListing {
	now := time.Now().In(timeZone)
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

func student_courses(w http.ResponseWriter, r *http.Request, student *StudentDB) {
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
		resp.Courses = append(resp.Courses, getCourseListing(student.Courses[tag], student))
	}

	writeJson(w, r, resp)
}

// get a list of assignments for this student with grade info
func student_grades(w http.ResponseWriter, r *http.Request, student *StudentDB) {
	courseTag := r.URL.Query().Get(":coursetag")

	course, present := student.Courses[courseTag]
	if !present {
		log.Printf("student not enrolled in course/course does not exist: %s", courseTag)
		http.Error(w, "course not found", http.StatusNotFound)
		return
	}

	list := []*AssignmentListing{}
	now := time.Now().In(timeZone)
	for _, asst := range course.Assignments {
		if !now.Before(asst.Open) {
			list = append(list, getAssignmentListing(asst, student))
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

func student_assignment(w http.ResponseWriter, r *http.Request, student *StudentDB) {
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
	now := time.Now().In(timeZone)
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
	problemType := problem.Type

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
		Assignment:  getAssignmentListing(asst, student),
		Attempt:     attempt,
	}

	writeJson(w, r, resp)
}

type StudentGraderReportResult struct {
	CourseTag   string
	CourseName  string
	ProblemType *ProblemType
	ProblemData map[string]interface{}
	Assignment  *AssignmentListing
	Attempt     map[string]interface{}
	ResultData  map[string]interface{}
}

func student_result(w http.ResponseWriter, r *http.Request, student *StudentDB) {
	id, err := strconv.ParseInt(r.URL.Query().Get(":id"), 10, 64)
	if err != nil {
		log.Printf("Bad ID: %s", r.URL.Query().Get(":id"))
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	n64, err := strconv.ParseInt(r.URL.Query().Get(":n"), 10, 64)
	if err != nil {
		log.Printf("Bad submission number: %s", r.URL.Query().Get(":n"))
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	n := int(n64)

	// find the assignment
	asst, present := assignmentsByID[id]
	if !present {
		log.Printf("No such assignment: %d", id)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// find the course
	course := asst.Course

	// make sure the course is active
	now := time.Now().In(timeZone)
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
	problemType := problem.Type

	// filter problem fields down to what the student is allowed to see
	data := make(map[string]interface{})
	for _, field := range problemType.FieldList {
		if value, present := problem.Data[field.Name]; present && field.Student == "view" {
			data[field.Name] = value
		}
	}

	// get the student attempt
	sol, present := student.SolutionsByAssignment[asst.ID]
	if !present {
		log.Printf("Student has not submitted a solution for this assignment")
		http.Error(w, "Submission not found", http.StatusNotFound)
		return

	}

	// make sure this is a valid submission number
	if n == -1 {
		n = len(sol.SubmissionsInOrder) - 1
	}
	if n < 0 || n >= len(sol.SubmissionsInOrder) {
		log.Printf("Invalid submission number: %d", n)
		http.Error(w, "Submission not found", http.StatusNotFound)
		return
	}

	// get the submission
	submission := sol.SubmissionsInOrder[n]

	resp := &StudentGraderReportResult{
		CourseTag:   course.Tag,
		CourseName:  course.Name,
		ProblemType: problemType,
		ProblemData: data,
		Assignment:  getAssignmentListing(asst, student),
		Attempt:     submission.Submission,
		ResultData:  submission.GradeReport,
	}

	writeJson(w, r, resp)
}

func student_submit(w http.ResponseWriter, r *http.Request, db *sql.DB, student *StudentDB, decoder *json.Decoder) {
	asstID, err := strconv.ParseInt(r.URL.Query().Get(":id"), 10, 64)
	if err != nil {
		log.Printf("Bad assignment ID: %s", r.URL.Query().Get(":id"))
		http.Error(w, "Assignment not found", http.StatusNotFound)
		return
	}

	data := make(map[string]interface{})
	if err := decoder.Decode(&data); err != nil {
		log.Printf("Failure decoding JSON request: %v", err)
		http.Error(w, "Failure decoding JSON request", http.StatusBadRequest)
		return
	}

	// make sure this assignment exists
	asst, present := assignmentsByID[asstID]
	if !present {
		log.Printf("No such assignment: %d", asstID)
		http.Error(w, "Assignment not found", http.StatusNotFound)
		return
	}

	// make sure the assignment is active
	now := time.Now().In(timeZone)
	if now.Before(asst.Open) || now.After(asst.Close) {
		log.Printf("Assignment is not active: %d", asstID)
		http.Error(w, "Assignment not active", http.StatusForbidden)
		return
	}

	// get the problem type description
	problemType := asst.Problem.Type

	// filter it down to expected student fields
	filtered := make(map[string]interface{})

	for _, field := range problemType.FieldList {
		if value, present := data[field.Name]; present && field.Student == "edit" {
			filtered[field.Name] = value
		} else if field.Student == "edit" {
			log.Printf("Missing %s field in submission", field.Name)
			http.Error(w, "Submission missing required field", http.StatusBadRequest)
			return
		}
	}

	// get the course
	course := asst.Course

	// make sure this is an active course
	if now.After(course.Close) {
		log.Printf("Not an active course: %s", course.Tag)
		http.Error(w, "Not an active course", http.StatusNotFound)
		return
	}

	// make sure the student is enrolled in the course
	if _, present := student.Courses[course.Tag]; !present {
		log.Printf("Not enrolled in the course: %s", course.Tag)
		http.Error(w, "Not enrolled in the course", http.StatusForbidden)
		return
	}

	txn, err := db.Begin()
	if err != nil {
		log.Printf("DB error starting transaction: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// default if we quit along the way is to rollback
	defer txn.Rollback()

	// is this the first submission for this assignment?
	solution, solutionPresent := student.SolutionsByAssignment[asstID]
	if !solutionPresent {
		result, err := txn.Exec("insert into Solution values (null, ?, ?)",
			student.Email,
			asst.ID)
		if err != nil {
			log.Printf("DB error inserting new Solution: %v", err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		id, err := result.LastInsertId()
		if err != nil {
			log.Printf("DB error getting ID for new Solution: %v", err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		solution = &SolutionDB{
			ID:                 id,
			Student:            student,
			Assignment:         asst,
			SubmissionsInOrder: []*SubmissionDB{},
		}
	}

	submissionJson, err := json.Marshal(filtered)
	if err != nil {
		log.Printf("JSON error encoding submission: %v", err)
		http.Error(w, "Encoding error", http.StatusInternalServerError)
		return
	}

	// create the submission
	_, err = txn.Exec("insert into Submission values (?, ?, ?, ?, ?)",
		solution.ID,
		now,
		submissionJson,
		"",
		false)
	if err != nil {
		log.Printf("DB insert error on Submission: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// commit the transaction
	if err = txn.Commit(); err != nil {
		log.Printf("DB commit error: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// add the solution to memory if needed
	if !solutionPresent {
		solutionsByID[solution.ID] = solution
		student.SolutionsByAssignment[asst.ID] = solution
		asst.SolutionsByStudent[student.Email] = solution
	}

	// add the submission to memory
	sub := &SubmissionDB{
		Solution:    solution,
		TimeStamp:   now,
		Submission:  filtered,
		GradeReport: make(map[string]interface{}),
		Passed:      false,
	}
	solution.SubmissionsInOrder = append(solution.SubmissionsInOrder, sub)

	// notify the grader of work to do
	notifyGrader <- solution.ID
}
