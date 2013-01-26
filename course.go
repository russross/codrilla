package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"github.com/gorilla/pat"
	"github.com/gorilla/sessions"
	"github.com/vmihailenco/redis"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func init() {
	r := pat.New()
	r.Add("POST", `/course/canvascsvlist`, handlerInstructor(course_canvascsvlist))
	r.Add("POST", `/course/canvasstudentmappings`, handlerInstructorJson(course_canvasstudentmappings))
	r.Add("GET", `/course/list`, handlerInstructor(course_list))
	r.Add("POST", `/course/newassignment/{coursetag:[\w:_\-]+$}`, handlerInstructorJson(course_newassignment))
	r.Add("GET", `/course/grades/{coursetag:[\w:_\-]+$}`, handlerInstructor(course_grades))
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

func course_list(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	email := session.Values["email"].(string)

	iface := db.EvalSha(luaScripts["courselist"], nil, []string{email})
	if iface.Err() != nil {
		log.Printf("DB error getting course listing for %s: %v", email, iface.Err())
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

type NewAssignment struct {
	Problem   int64
	Open      int64
	Close     int64
	ForCredit bool
}

func course_newassignment(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session, decoder *json.Decoder) {
	email := session.Values["email"].(string)
	courseTag := r.URL.Query().Get(":coursetag")

	asst := new(NewAssignment)
	if err := decoder.Decode(asst); err != nil {
		log.Printf("Failure decoding JSON request: %v", err)
		http.Error(w, "Failure decoding JSON request", http.StatusBadRequest)
		return
	}

	// sanity check the problem number
	if asst.Problem < 1 {
		log.Printf("Problem number must be > 0")
		http.Error(w, "Invalid problem number", http.StatusBadRequest)
		return
	}
	problem := strconv.FormatInt(asst.Problem, 10)

	// if the open time is missing, use now
	now := time.Now().Unix()
	if asst.Open <= 0 {
		asst.Open = now
	}

	// it must not open in the past
	if asst.Open < now {
		log.Printf("Open time must be in the future")
		http.Error(w, "Open time must be in the future", http.StatusBadRequest)
		return
	}
	open := strconv.FormatInt(asst.Open, 10)

	// it must not close in the past, or before it opens
	if asst.Close < now || asst.Close <= asst.Open {
		log.Printf("Must close in the future after opening")
		http.Error(w, "Close time must be in the future and after open time", http.StatusBadRequest)
		return
	}
	closeTime := strconv.FormatInt(asst.Close, 10)
	forCredit := strconv.FormatBool(asst.ForCredit)

	iface := db.EvalSha(luaScripts["newassignment"], nil, []string{email, courseTag, problem, open, closeTime, forCredit})
	if iface.Err() != nil {
		log.Printf("DB error creating new assignment for %s: %v", email, iface.Err())
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}
}

func course_grades(w http.ResponseWriter, r *http.Request, db *redis.Client, session *sessions.Session) {
	email := session.Values["email"].(string)
	courseTag := r.URL.Query().Get(":coursetag")

	iface := db.EvalSha(luaScripts["coursegrades"], nil, []string{email, courseTag})
	if iface.Err() != nil {
		log.Printf("DB error getting course %s grades for %s: %v", courseTag, email, iface.Err())
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
