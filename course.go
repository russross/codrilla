package main

import (
	"database/sql"
	"encoding/json"
	"github.com/gorilla/pat"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

func init() {
	r := pat.New()
	r.Add("GET", `/course/list`, handlerInstructor(course_list))
	r.Add("GET", `/course/grades/{coursetag:[\w:_\-]+$}`, handlerInstructor(course_grades))
	r.Add("POST", `/course/newassignment/{coursetag:[\w:_\-]+$}`, handlerInstructorJson(course_newassignment))
	r.Add("POST", `/course/courselistupload/{coursetag:[\w:_\-]+$}`, handlerInstructorJson(course_courselistupload))
	http.Handle("/course/", r)
}

func course_courselistupload(w http.ResponseWriter, r *http.Request, db *sql.DB, instructor *InstructorDB, decoder *json.Decoder) {
	courseTag := r.URL.Query().Get(":coursetag")
	course, present := instructor.Courses[courseTag]
	if !present {
		log.Printf("Not an instructor for %s/course does not exist", courseTag)
		http.Error(w, "Course not found", http.StatusNotFound)
		return
	}

	now := time.Now().In(timeZone)
	if now.After(course.Close) {
		log.Printf("Course is closed")
		http.Error(w, "Course is closed", http.StatusForbidden)
		return
	}

	lst := [][]string{}
	if err := decoder.Decode(&lst); err != nil {
		log.Printf("Error decoding list of students: %v", err)
		http.Error(w, "Error decoding list of students", http.StatusBadRequest)
		return
	}

	if len(lst) == 0 {
		log.Printf("Course cannot be populated with empty list")
		http.Error(w, "Course cannot be populated with empty list", http.StatusBadRequest)
		return
	}

	// validate the data
	studentsToAdd := make(map[string]string)
	studentsToRemove := make(map[string]bool)
	for _, row := range lst {
		if len(row) != 2 {
			log.Printf("Row with wrong number of elements: %d instead of 2", len(row))
			http.Error(w, "Data row of wrong size", http.StatusBadRequest)
			return
		}
		row[0] = strings.TrimSpace(row[0])
		row[1] = strings.ToLower(strings.TrimSpace(row[1]))
		if len(row[0]) == 0 || len(row[1]) == 0 {
			log.Printf("Row found with empty data")
			http.Error(w, "Row found with empty data", http.StatusBadRequest)
			return
		}
		if !strings.ContainsRune(row[1], '@') {
			row[1] += config.StudentEmailDomain
		}

		name, email := row[0], row[1]
		studentsToAdd[email] = name
	}

	// figure out who to remove
	for _, student := range course.Students {
		if _, present := studentsToAdd[student.Email]; !present {
			studentsToRemove[student.Email] = true
		}
	}

	// looks good, so start updating
	txn, err := db.Begin()
	if err != nil {
		log.Printf("DB error starting transaction: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	defer txn.Rollback()

	// add/update students records
	for email, name := range studentsToAdd {
		student, present := studentsByEmail[email]
		if !present {
			if _, err := txn.Exec("insert into Student values (?, ?)", email, name); err != nil {
				log.Printf("DB error inserting Student: %v", err)
				http.Error(w, "DB error", http.StatusInternalServerError)
				return
			}
		} else if student.Name != name {
			if _, err := txn.Exec("update Student set Name = ? where Email = ?", name, email); err != nil {
				log.Printf("DB error updating Student: %v", err)
				http.Error(w, "DB error", http.StatusInternalServerError)
				return
			}
		}

		// add student to course if not already enrolled
		if _, present = course.Students[email]; !present {
			if _, err := txn.Exec("insert into CourseStudent values (?, ?)", course.Tag, email); err != nil {
				log.Printf("DB error inserting CourseStudent: %v", err)
				http.Error(w, "DB error", http.StatusInternalServerError)
				return
			}
		}
	}

	// remove student records from course
	for email, _ := range studentsToRemove {
		if _, err := txn.Exec("delete from CourseStudent where Course = ? and Student = ?", course.Tag, email); err != nil {
			log.Printf("DB error delete from CourseStudent: %v", err)
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
	}

	// commit
	if err = txn.Commit(); err != nil {
		log.Printf("DB error committing: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// now update in-memory structures
	// add/update students
	for email, name := range studentsToAdd {
		student, present := studentsByEmail[email]
		if !present {
			student = &StudentDB{
				Email:                 email,
				Name:                  name,
				Courses:               make(map[string]*CourseDB),
				SolutionsByAssignment: make(map[int64]*SolutionDB),
			}
			studentsByEmail[email] = student
		}
		if student.Name != name {
			student.Name = name
		}
		student.Courses[course.Tag] = course
		course.Students[email] = student
	}

	// delete students who have dropped
	for email, _ := range studentsToRemove {
		student := course.Students[email]
		delete(course.Students, email)
		delete(student.Courses, course.Tag)
	}
}

type CourseListResponseElt struct {
	Tag               string
	Name              string
	Close             time.Time
	Instructors       []string
	Students          []string
	OpenAssignments   []*AssignmentListing
	ClosedAssignments []*AssignmentListing
	FutureAssignments []*AssignmentListing
}

func course_list(w http.ResponseWriter, r *http.Request, instructor *InstructorDB) {
	resp := []*CourseListResponseElt{}
	now := time.Now().In(timeZone)

	// get list of active course names in sorted order
	courses := []string{}
	for _, course := range instructor.Courses {
		if now.After(course.Close) {
			continue
		}
		courses = append(courses, course.Tag)
	}
	sort.Strings(courses)

	// process courses one at a time
	for _, courseName := range courses {
		course := instructor.Courses[courseName]
		elt := &CourseListResponseElt{
			Tag:               course.Tag,
			Name:              course.Name,
			Close:             course.Close,
			Instructors:       []string{},
			Students:          []string{},
			OpenAssignments:   []*AssignmentListing{},
			ClosedAssignments: []*AssignmentListing{},
			FutureAssignments: []*AssignmentListing{},
		}

		// get instructors
		for email, _ := range course.Instructors {
			elt.Instructors = append(elt.Instructors, email)
		}
		sort.Strings(elt.Instructors)

		// get students
		for email, _ := range course.Students {
			elt.Students = append(elt.Students, email)
		}
		sort.Strings(elt.Students)

		// get assignments
		for _, asst := range course.Assignments {
			if now.After(asst.Close) {
				elt.ClosedAssignments = append(elt.ClosedAssignments, getAssignmentListing(asst, nil))
			} else if now.Before(asst.Open) {
				elt.FutureAssignments = append(elt.FutureAssignments, getAssignmentListing(asst, nil))
			} else {
				elt.OpenAssignments = append(elt.OpenAssignments, getAssignmentListing(asst, nil))
			}
		}
		sort.Sort(AssignmentsByClose(elt.OpenAssignments))
		sort.Sort(AssignmentsByClose(elt.ClosedAssignments))
		sort.Sort(AssignmentsByOpen(elt.FutureAssignments))

		resp = append(resp, elt)
	}

	writeJson(w, r, resp)
}

type NewAssignment struct {
	Problem   int64
	Open      time.Time
	Close     time.Time
	ForCredit bool
}

func course_newassignment(w http.ResponseWriter, r *http.Request, db *sql.DB, instructor *InstructorDB, decoder *json.Decoder) {
	courseTag := r.URL.Query().Get(":coursetag")
	course, present := instructor.Courses[courseTag]
	if !present {
		log.Printf("Not an instructor for %s/course does not exist", courseTag)
		http.Error(w, "Course not found", http.StatusNotFound)
		return
	}

	now := time.Now().In(timeZone)
	if now.After(course.Close) {
		log.Printf("Course %s is closed", courseTag)
		http.Error(w, "Course is closed", http.StatusForbidden)
		return
	}

	asst := new(NewAssignment)
	if err := decoder.Decode(asst); err != nil {
		log.Printf("Failure decoding JSON request: %v", err)
		http.Error(w, "Failure decoding JSON request", http.StatusBadRequest)
		return
	}

	// get the problem
	problem, present := problemsByID[asst.Problem]
	if !present {
		log.Printf("Problem %d not found", asst.Problem)
		http.Error(w, "Problem not found", http.StatusNotFound)
		return
	}

	// if the open time is missing, use now
	if asst.Open.IsZero() || asst.Open.Year() < 2000 {
		asst.Open = now
	}

	// it must not open in the past
	if now.After(asst.Open) && !now.Equal(asst.Open) {
		log.Printf("Open time must be in the future")
		http.Error(w, "Open time must be in the future", http.StatusBadRequest)
		return
	}

	// it must not close in the past, or before it opens
	if now.After(asst.Close) || asst.Close.Before(asst.Open) {
		log.Printf("Must close in the future after opening")
		http.Error(w, "Close time must be in the future and after open time", http.StatusBadRequest)
		return
	}

	// write to the database first
	result, err := db.Exec("insert into Assignment values (null, ?, ?, ?, ?, ?)",
		course.Tag,
		problem.ID,
		asst.ForCredit,
		asst.Open,
		asst.Close)
	if err != nil {
		log.Printf("DB error inserting new Assignment: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		log.Printf("DB error getting ID of new Assignment: %v", err)
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// up date in-memory data structures
	elt := &AssignmentDB{
		ID:                 id,
		Course:             course,
		Problem:            problem,
		ForCredit:          asst.ForCredit,
		Open:               asst.Open,
		Close:              asst.Close,
		SolutionsByStudent: make(map[string]*SolutionDB),
	}
	assignmentsByID[id] = elt
	course.Assignments[id] = elt
	problem.Assignments[id] = elt
}

type CourseGradesResponseElt struct {
	Name        string
	Email       string
	Assignments []*AssignmentListing
}

func course_grades(w http.ResponseWriter, r *http.Request, instructor *InstructorDB) {
	courseTag := r.URL.Query().Get(":coursetag")
	course, present := coursesByTag[courseTag]
	if !present {
		log.Printf("No such course/not an instructor for course %s", courseTag)
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// get a list of students in sorted order
	resp := []*CourseGradesResponseElt{}
	order := []string{}
	for email, _ := range course.Students {
		order = append(order, email)
	}
	sort.Strings(order)

	// process each student
	now := time.Now().In(timeZone)
	for _, email := range order {
		student := course.Students[email]
		elt := &CourseGradesResponseElt{
			Email:       student.Email,
			Name:        student.Name,
			Assignments: []*AssignmentListing{},
		}
		for _, asst := range course.Assignments {
			if now.Before(asst.Close) {
				elt.Assignments = append(elt.Assignments, getAssignmentListing(asst, student))
			}
		}
		sort.Sort(AssignmentsByClose(elt.Assignments))

		resp = append(resp, elt)
	}

	writeJson(w, r, resp)
}
