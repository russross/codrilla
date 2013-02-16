package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/gorilla/pat"
	"github.com/russross/blackfriday"
	"log"
	"net/http"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"time"
)

func init() {
	r := pat.New()
	r.Add("GET", `/student/courses`, handlerStudent(student_courses))
	r.Add("GET", `/student/assignment/{id:\d+$}`, handlerStudent(student_assignment))
	r.Add("GET", `/student/submission/{id:\d+}/{n:\d+$}`, handlerStudent(student_assignment))
	r.Add("GET", `/student/download/{id:\d+$}`, handlerStudent(student_download))
	r.Add("POST", `/student/submit/{id:\d+$}`, handlerStudentJson(student_submit))
	http.Handle("/student/", r)
}

type CourseListing struct {
	Tag               string
	Name              string
	Close             time.Time
	Instructors       []string
	PastAssignments   []*AssignmentListing
	OpenAssignments   []*AssignmentListing
	FutureAssignments []*AssignmentListing
}

func getCourseListing(course *CourseDB, student *StudentDB) *CourseListing {
	now := time.Now().In(timeZone)
	elt := &CourseListing{
		Tag:               course.Tag,
		Name:              course.Name,
		Close:             course.Close,
		Instructors:       []string{},
		PastAssignments:   []*AssignmentListing{},
		OpenAssignments:   []*AssignmentListing{},
		FutureAssignments: []*AssignmentListing{},
	}
	for email, _ := range course.Instructors {
		elt.Instructors = append(elt.Instructors, email)
	}
	for _, asst := range course.Assignments {
		if now.After(asst.Close) {
			elt.PastAssignments = append(elt.PastAssignments, getAssignmentListing(asst, student))
		} else if now.Before(asst.Open) {
			elt.FutureAssignments = append(elt.FutureAssignments, getAssignmentListing(asst, student))
		} else {
			elt.OpenAssignments = append(elt.OpenAssignments, getAssignmentListing(asst, student))
		}
	}
	sort.Sort(AssignmentsByClose(elt.OpenAssignments))
	sort.Sort(AssignmentsByClose(elt.PastAssignments))
	sort.Sort(AssignmentsByOpen(elt.FutureAssignments))

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

func (p AssignmentsByOpen) Len() int { return len(p) }
func (p AssignmentsByOpen) Less(i, j int) bool {
	if p[i].Open.Equal(p[j].Open) {
		return p[i].Name < p[j].Name
	}
	return p[i].Open.Before(p[j].Open)
}
func (p AssignmentsByOpen) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

type AssignmentsByClose []*AssignmentListing

func (p AssignmentsByClose) Len() int { return len(p) }
func (p AssignmentsByClose) Less(i, j int) bool {
	if p[i].Close.Equal(p[j].Close) {
		return p[i].Name < p[j].Name
	}
	return p[i].Close.Before(p[j].Close)
}
func (p AssignmentsByClose) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

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

type StudentAssignmentResult struct {
	CourseTag   string
	CourseName  string
	ProblemType *ProblemType
	Assignment  *AssignmentListing
	Data        map[string]interface{}
}

func getStudentAssignmentData(w http.ResponseWriter, r *http.Request, student *StudentDB, id int64, n int) (*CourseDB, *AssignmentDB, map[string]interface{}) {
	// find the assignment
	asst, present := assignmentsByID[id]
	if !present {
		log.Printf("No such assignment: %d", id)
		http.Error(w, "Not found", http.StatusNotFound)
		return nil, nil, nil
	}

	// make sure the assignment is active or past
	now := time.Now().In(timeZone)
	if now.Before(asst.Open) {
		log.Printf("Assignment is not yet open: %d", asst.ID)
		http.Error(w, "Assignment not open yet", http.StatusForbidden)
		return nil, nil, nil
	}

	// find the course
	course := asst.Course

	// make sure the course is active
	if now.After(course.Close) {
		log.Printf("Course is not active: %s", course.Tag)
		http.Error(w, "Course not active", http.StatusForbidden)
		return nil, nil, nil
	}

	// make sure the student is in the course
	if _, present := student.Courses[course.Tag]; !present {
		log.Printf("Student not enrolled in course: %s", course.Tag)
		http.Error(w, "Not enrolled in course", http.StatusForbidden)
		return nil, nil, nil
	}

	// get the problem
	problem := asst.Problem
	problemType := problem.Type

	// filter problem fields down to what the student is allowed to see
	data := filterFields("result", "view", problemType, problem.Data)

	// get the student attempt
	sol, present := student.SolutionsByAssignment[asst.ID]
	count := 0
	if present {
		count = len(sol.SubmissionsInOrder)
	}
	if n == -1 && count > 0 {
		n = count - 1
	}
	if n != -1 && (n < 0 || n >= count) {
		log.Printf("Invalid solution number requested: %d with %d available", n, count)
		http.Error(w, "Submission not found", http.StatusNotFound)
		return nil, nil, nil
	}

	// get the requested submission
	if count > 0 {
		submission := sol.SubmissionsInOrder[n]
		attempt := filterFields("student", "edit", problemType, submission.Submission)
		for key, value := range attempt {
			data[key] = value
		}
		if len(submission.GradeReport) > 0 {
			report := filterFields("grader", "edit", problemType, submission.GradeReport)
			for _, field := range problemType.FieldList {
				if value, present := report[field.Name]; present && field.Result == "view" {
					data[field.Name] = value
				}
			}
		}
	} else {
		data["Report"] = ""
	}

	// include the expected output if available
	output, err := getOutput(problem)
	if err == nil {
		data["Output"] = output
	}

	return course, asst, data
}

func student_assignment(w http.ResponseWriter, r *http.Request, student *StudentDB) {
	id, err := strconv.ParseInt(r.URL.Query().Get(":id"), 10, 64)
	if err != nil {
		log.Printf("Bad ID: %s", r.URL.Query().Get(":id"))
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}
	n_s := r.URL.Query().Get(":n")
	n := -1
	if n_s != "" {
		n64, err := strconv.ParseInt(n_s, 10, 64)
		if err != nil || n < 0 {
			log.Printf("Bad submission number: %s", n_s)
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}
		n = int(n64)
	}

	// get the data to return
	course, asst, data := getStudentAssignmentData(w, r, student, id, n)
	if data == nil || len(data) == 0 {
		return
	}

	resp := &StudentAssignmentResult{
		CourseTag:   course.Tag,
		CourseName:  course.Name,
		ProblemType: asst.Problem.Type,
		Assignment:  getAssignmentListing(asst, student),
		Data:        data,
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
	filtered := filterFields("student", "edit", problemType, data)

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

func student_download(w http.ResponseWriter, r *http.Request, student *StudentDB) {
	id, err := strconv.ParseInt(r.URL.Query().Get(":id"), 10, 64)
	if err != nil {
		log.Printf("Bad ID: %s", r.URL.Query().Get(":id"))
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	// get the data to download
	_, asst, data := getStudentAssignmentData(w, r, student, id, -1)
	if data == nil || len(data) == 0 {
		return
	}

	filename, zipfile, err := makeProblemZipFile(asst.Problem, data)
	if err != nil {
		http.Error(w, "Failed to create zipfile", http.StatusInternalServerError)
	}

	w.Header()["Content-Type"] = []string{"application/zip"}
	w.Header()["Content-Length"] = []string{fmt.Sprintf("%d", len(zipfile))}
	w.Header()["Content-Disposition"] =
		[]string{`attachment; filename="` + filename + `"`}
	w.Write(zipfile)
}

var (
	Apostrophe          = regexp.MustCompile(`'`)
	NonWord             = regexp.MustCompile(`[^\w\.]+`)
	Underscores         = regexp.MustCompile(`__+`)
	LeadTrailUnderscore = regexp.MustCompile(`^_|_$`)
	LonelyS1            = regexp.MustCompile(`_s_`)
	LonelyS2            = regexp.MustCompile(`_s$`)
)

func makeProblemZipFile(problem *ProblemDB, data map[string]interface{}) (filename string, zipfile []byte, err error) {
	problemType := problem.Type

	// make the problem name into a decent directory name
	prefix := problem.Name
	prefix = Apostrophe.ReplaceAllString(prefix, "")
	prefix = NonWord.ReplaceAllString(prefix, "_")
	prefix = Underscores.ReplaceAllString(prefix, "_")
	prefix = LeadTrailUnderscore.ReplaceAllString(prefix, "")
	prefix = LonelyS1.ReplaceAllString(prefix, "s_")
	prefix = LonelyS2.ReplaceAllString(prefix, "s")

	var buf bytes.Buffer
	z := zip.NewWriter(&buf)

	// extract the appropriate fields
	for _, field := range problemType.FieldList {
		value, present := data[field.Name]
		if !present {
			continue
		}

		// gather the value or values into a list
		var values []interface{}
		if field.List {
			if lst, ok := value.([]interface{}); ok {
				values = lst
			} else {
				log.Printf("makeProblemZipFile expected []interface{} from %s but found %T", field.Name, value)
			}
		} else {
			values = []interface{}{value}
		}

		var name string
		for i, elt := range values {
			s := fmt.Sprintf("%v", elt)
			switch field.Type {
			case "string", "text":
				if field.List {
					name = fmt.Sprintf("%s%02d.txt", field.Name, i+1)
				} else {
					name = fmt.Sprintf("%s.txt", field.Name)
				}

			case "python":
				if field.List {
					name = fmt.Sprintf("%s%02d.py", field.Name, i+1)
				} else {
					name = fmt.Sprintf("%s.py", field.Name)
				}

			case "markdown":
				if field.List {
					name = fmt.Sprintf("%s%02d.html", field.Name, i+1)
				} else {
					name = fmt.Sprintf("%s.html", field.Name)
				}

				input := []byte("# " + problem.Name + "\n\n" + s)

				htmlFlags := 0
				htmlFlags |= blackfriday.HTML_USE_SMARTYPANTS
				htmlFlags |= blackfriday.HTML_SMARTYPANTS_FRACTIONS
				htmlFlags |= blackfriday.HTML_SMARTYPANTS_LATEX_DASHES
				htmlFlags |= blackfriday.HTML_COMPLETE_PAGE
				renderer := blackfriday.HtmlRenderer(
					htmlFlags,
					problem.Name,
					"http://codrilla.cs.dixie.edu/css/codrilla.css")

				extensions := 0
				extensions |= blackfriday.EXTENSION_NO_INTRA_EMPHASIS
				extensions |= blackfriday.EXTENSION_TABLES
				extensions |= blackfriday.EXTENSION_FENCED_CODE
				extensions |= blackfriday.EXTENSION_AUTOLINK
				extensions |= blackfriday.EXTENSION_STRIKETHROUGH
				extensions |= blackfriday.EXTENSION_SPACE_HEADERS
				output := blackfriday.Markdown(input, renderer, extensions)

				s = string(output)
			case "int", "bool":
				continue

			default:
				if field.List {
					name = fmt.Sprintf("%s%02d", field.Name, i+1)
				} else {
					name = fmt.Sprintf("%s", field.Name)
				}
			}

			if len(s) > 0 && s != "\n" {
				out, err := z.Create(filepath.Join(prefix, name))
				if err != nil {
					log.Printf("Error creating file %s in .zip file: %v", name, err)
					return "", nil, err
				}
				if _, err = out.Write([]byte(s)); err != nil {
					log.Printf("Error writing data to file %s in .zip file: %v", name, err)
					return "", nil, err
				}
			}
		}
	}
	if err = z.Close(); err != nil {
		log.Printf("Error closing .zip file: %v", err)
		return "", nil, err
	}

	return prefix + ".zip", buf.Bytes(), nil
}
