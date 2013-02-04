package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/gorilla/pat"
	"github.com/gorilla/sessions"
	"github.com/vmihailenco/redis"
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
	//r.Add("POST", `/course/canvascsvlist`, handlerInstructor(course_canvascsvlist))
	//r.Add("POST", `/course/canvasstudentmappings`, handlerInstructorJson(course_canvasstudentmappings))
	r.Add("POST", `/course/newassignment/{coursetag:[\w:_\-]+$}`, handlerInstructorJson(course_newassignment))
	http.Handle("/course/", r)
}

type CSVStudent struct {
	Name  string
	Email string
}

type CSVUploadResult struct {
	Success         bool
	UnknownStudents []string
	Log             []string
}

func course_canvascsvlist(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	instructor := session.Values["email"].(string)
	role := session.Values["email"].(string)

	result := &CSVUploadResult{
		Success:         true,
		UnknownStudents: []string{},
		Log:             []string{},
	}

	var records [][]string

	// parse the csv file
	for _, field := range []string{"CSVFile1", "CSVFile2", "CSVFile3"} {
		file, _, err := r.FormFile(field)
		if err != nil {
			continue
		}
		defer file.Close()

		reader := csv.NewReader(file)
		reader.TrailingComma = true
		list, err := reader.ReadAll()
		if err != nil {
			log.Printf("Error parsing CSV file: %v", err)
			http.Error(w, "Error parsing CSV file", http.StatusBadRequest)
			return
		}

		// make sure there was at least one student after skipping headers
		if len(list) < 3 {
			log.Printf("File does not seem to contain any students")
			continue
		}

		// throw away the header lines
		records = append(records, list[2:]...)
	}
	if len(records) == 0 {
		log.Printf("called with no CSV files or empty files")
		http.Error(w, "No records found", http.StatusBadRequest)
		return
	}

	// scan the list to see if we recognize the course and all the students
	course := ""
	for _, student := range records {
		if len(student) < 3 {
			log.Printf("Student record with too few fields: %v", student)
			http.Error(w, "Student record with too few fields", http.StatusBadRequest)
			return
		}
		name, id, section := student[0], student[1], student[2]

		// make sure this is a known course
		b := db.HExists("index:courses:tagbycanvastag", section)
		if b.Err() != nil {
			log.Printf("DB error checking for course tag for %s: %v", section, b.Err())
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		tag := ""
		if b.Val() {
			// get the course tag
			str := db.HGet("index:courses:tagbycanvastag", section)
			if str.Err() != nil {
				log.Printf("DB error getting course tag for %s: %v", section, str.Err())
				http.Error(w, "DB error", http.StatusInternalServerError)
				return
			}
			tag = str.Val()

			// check if the course is active
			if b = db.SIsMember("index:courses:active", tag); b.Err() != nil {
				log.Printf("DB error checking if course is active for %s: %v", tag, b.Err())
				http.Error(w, "DB error", http.StatusInternalServerError)
				return
			}
			if !b.Val() {
				log.Printf("Course is not active: %s", tag)
				http.Error(w, "Course is not active", http.StatusBadRequest)
				return
			}
		} else {
			// request the Canvas section -> tag name be filled in
			log.Printf("Mapping for course [%s] is unknown", section)
			http.Error(w, "Unknown course", http.StatusBadRequest)
			return
		}

		if course == "" && tag != "" {
			course = tag
		} else if course != "" && tag != "" && tag != course {
			log.Printf("Error: two courses found, %s and %s", course, tag)
			http.Error(w, "Files contain data for more than one course", http.StatusBadRequest)
			return
		}

		// see if we know the email address for this student
		if b = db.HExists("index:students:emailbyid", id); b.Err() != nil {
			log.Printf("DB error checking for student ID %s for student %s: %v", id, name, b.Err())
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		if !b.Val() {
			result.Success = false
			result.UnknownStudents = append(result.UnknownStudents, fmt.Sprintf("%s (%s)", name, id))
		}
	}

	// is this instructor over this course?
	if course != "" && role != "admin" {
		b := db.SIsMember("course:"+course+":instructors", instructor)
		if b.Err() != nil {
			log.Printf("DB error checking if %s is an instructor for %s: %v", instructor, course, b.Err())
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}

		if !b.Val() {
			log.Printf("%s is not an instructor for %s", instructor, course)
			http.Error(w, "You are not an instructor for this course", http.StatusUnauthorized)
			return
		}
	}

	// do we need more info before continuing?
	if !result.Success {
		writeJson(w, r, result)
		return
	}

	if course == "" {
		log.Printf("Error: no course found")
		http.Error(w, "No course found", http.StatusBadRequest)
		return
	}

	// now loop through again and set the students
	emailToName := make(map[string]string)
	for _, student := range records {
		name, id := student[0], student[1]

		// get the email address
		str := db.HGet("index:students:emailbyid", id)
		if str.Err() != nil {
			log.Printf("DB error getting student email address for %s (%s): %v", name, id, str.Err())
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		email := str.Val()

		// note the students we have found
		emailToName[email] = name

		// add this student to the course
		iface := db.EvalSha(luaScripts["addstudenttocourse"], []string{}, []string{email, name, course})
		if iface.Err() != nil {
			log.Printf("DB error adding student to course: %v", iface.Err())
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}

		if iface.Val().(string) == "noop" {
			result.Log = append(result.Log, fmt.Sprintf("Student %s (%s) already in %s", name, email, course))
		} else {
			result.Log = append(result.Log, fmt.Sprintf("Student %s (%s) added to %s", name, email, course))
		}
	}

	// check for students that were not in the list
	slice := db.SMembers("course:" + course + ":students")
	if slice.Err() != nil {
		log.Printf("DB error getting list of students in course %s: %v", course, slice.Err())
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
	roll := slice.Val()
	for _, elt := range roll {
		if name, present := emailToName[elt]; !present {
			// drop the student
			iface := db.EvalSha(luaScripts["dropstudent"], []string{}, []string{elt, course})
			if iface.Err() != nil {
				log.Printf("DB error dropping student %s frop course %s: %v", elt, course, iface.Err())
				http.Error(w, "DB error", http.StatusInternalServerError)
				return
			}
			result.Log = append(result.Log, fmt.Sprintf("Student %s (%s) dropped from %s", name, elt, course))
		}
	}

	writeJson(w, r, result)
}

func course_canvasstudentmappings(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session, decoder *json.Decoder) {
	mappings := make(map[string]string)
	if err := decoder.Decode(&mappings); err != nil {
		log.Printf("Failure decoding JSON request: %v", err)
		http.Error(w, "Failure decoding JSON request", http.StatusBadRequest)
		return
	}

	// add canvas student id -> email mappings
	for canvas, email := range mappings {
		if !strings.ContainsRune(email, '@') {
			email += config.StudentEmailDomain
		}

		// set the mapping if it does not already exist
		b := db.HSetNX("index:students:emailbyid", canvas, email)
		if b.Err() != nil {
			log.Printf("DB error setting student mapping: %v", b.Err())
			http.Error(w, "DB error", http.StatusInternalServerError)
			return
		}
		if b.Val() {
			log.Printf("Mapping set for Canvas student %s -> %s", canvas, email)
		} else {
			log.Printf("Mapping already exists for student %s (%s), skipping", canvas, email)
		}
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
	now := time.Now()

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
	Open      int64
	Close     int64
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

	now := time.Now()
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
	open := now
	if asst.Open > 0 {
		open = time.Unix(asst.Open, 0)
	}

	// it must not open in the past
	if now.After(open) {
		log.Printf("Open time must be in the future")
		http.Error(w, "Open time must be in the future", http.StatusBadRequest)
		return
	}

	// it must not close in the past, or before it opens
	closeTime := time.Unix(asst.Close, 0)
	if now.After(closeTime) || closeTime.Before(open) {
		log.Printf("Must close in the future after opening")
		http.Error(w, "Close time must be in the future and after open time", http.StatusBadRequest)
		return
	}

	// write to the database first
	result, err := db.Exec("insert into Assignment values (null, ?, ?, ?, ?, ?)",
		course.Tag,
		problem.ID,
		asst.ForCredit,
		open,
		closeTime)
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
		Open:               open,
		Close:              closeTime,
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
	now := time.Now()
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
